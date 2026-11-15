package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/gossip"
)

const twoPi = 2 * math.Pi

type config struct {
	id      string
	stepper core.Stepper
	omega   float64
	dt      float64
	phase   core.Phase
	sink    string
	peers   gossip.PeerSource
	ticks   int
	wait    time.Duration
	log     *slog.Logger
	onPhase func(float64)
}

func (c config) logger() *slog.Logger {
	if c.log != nil {
		return c.log
	}
	return slog.Default()
}

// node is one fleet member. The receive loop and the tick loop share the
// latest phase heard from each id.
type node struct {
	cfg config
	tr  gossip.Transport

	mu      sync.Mutex
	phase   core.Phase
	heard   map[string]core.Phase
	seq     uint64
	started time.Time
}

func (n *node) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	n.phase = n.cfg.phase
	n.heard = map[string]core.Phase{}
	n.started = time.Now()

	errc := make(chan error, 1)
	go func() {
		err := n.receive(ctx)
		if parent.Err() == nil {
			cancel()
		}
		errc <- err
	}()

	tickErr := n.drive(ctx)
	cancel()
	recvErr := <-errc

	switch {
	case tickErr != nil && !errors.Is(tickErr, context.Canceled):
		return tickErr
	case recvErr != nil && !errors.Is(recvErr, context.Canceled):
		return recvErr
	case parent.Err() != nil:
		return parent.Err()
	default:
		return nil
	}
}

func (n *node) receive(ctx context.Context) error {
	for {
		_, msg, err := n.tr.Receive(ctx)
		if err != nil {
			return err
		}
		if msg.ID == "" || msg.ID == n.cfg.id {
			continue
		}
		n.mu.Lock()
		n.heard[msg.ID] = msg.Phase
		n.mu.Unlock()
	}
}

func (n *node) drive(ctx context.Context) error {
	n.notePhase()
	if err := n.publish(ctx); err != nil {
		return err
	}
	for tick := 0; n.cfg.ticks == 0 || tick < n.cfg.ticks; tick++ {
		if err := wait(ctx, n.cfg.wait); err != nil {
			return err
		}
		if err := n.step(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) step(ctx context.Context) error {
	n.mu.Lock()
	obs := make([]core.Observation, 0, len(n.heard))
	for id, phase := range n.heard {
		obs = append(obs, core.Observation{ID: id, Phase: phase})
	}
	prev := n.phase
	n.mu.Unlock()

	next := n.cfg.stepper.Step(prev, obs, n.cfg.dt)
	didFire := fired(prev, next, n.cfg.omega, n.cfg.dt)

	n.mu.Lock()
	n.phase = next
	n.mu.Unlock()
	n.notePhase()

	if err := n.publish(ctx); err != nil {
		return err
	}
	n.cfg.logger().Info("tick",
		"id", n.cfg.id,
		"phase", float64(next),
		"neighbors", len(obs),
		"fired", didFire,
	)
	if !didFire || n.cfg.sink == "" {
		return nil
	}
	if err := postFire(ctx, n.cfg.sink, n.cfg.id); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n.cfg.logger().Warn("sink", "id", n.cfg.id, "err", err)
	}
	return nil
}

func (n *node) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n.mu.Lock()
	n.seq++
	msg := gossip.Message{
		ID:     n.cfg.id,
		Phase:  n.phase,
		Seq:    n.seq,
		SentAt: time.Now(),
	}
	n.mu.Unlock()

	if n.cfg.peers == nil {
		return nil
	}
	list, err := n.cfg.peers.Peers(ctx)
	if err != nil {
		n.cfg.logger().Warn("peers", "id", n.cfg.id, "err", err)
		return nil
	}
	for _, peer := range list {
		if peer.ID == n.cfg.id {
			continue
		}
		if err := n.tr.Send(ctx, peer, msg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n.cfg.logger().Warn("send", "id", n.cfg.id, "to", peer.ID, "err", err)
		}
	}
	return nil
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// fired reports a forward crossing of phase 0. The coupling piece is the
// shortest step from the freely rotated phase to next, matching the simulator.
func fired(prev, next core.Phase, omega, dt float64) bool {
	if dt <= 0 {
		return false
	}
	base := core.Wrap(float64(prev) + omega*dt)
	add := omega*dt + core.Delta(base, next)
	if add <= 1e-12 {
		return false
	}
	start := float64(prev)
	boundary := (math.Floor(start/twoPi) + 1) * twoPi
	return boundary <= start+add+1e-9
}

func (n *node) notePhase() {
	if n.cfg.onPhase == nil {
		return
	}
	n.mu.Lock()
	phase := n.phase
	n.mu.Unlock()
	n.cfg.onPhase(phaseOffset(phase, n.cfg.omega, time.Since(n.started)))
}

func postFire(ctx context.Context, sink, id string) error {
	target, err := fireURL(sink)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	body, err := json.Marshal(struct {
		ID string `json:"id"`
	}{ID: id})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sink status %s", resp.Status)
	}
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// fireURL posts to /fire when the flag is only a host. A path that is
// already set, including /fire, is kept.
func fireURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("sink %q must be an http url", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/fire"
	}
	return u.String(), nil
}

// phaseOfID is the default initial phase. The mix matches the simulator's
// hash offset, so members with different ids do not all start together.
func phaseOfID(id string) core.Phase {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	z := h.Sum64()
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z = z ^ (z >> 31)
	return core.Wrap((float64(z) / math.MaxUint64) * twoPi)
}
