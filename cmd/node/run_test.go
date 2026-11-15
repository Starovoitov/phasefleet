package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/gossip"
)

func TestTwoNodesConverge(t *testing.T) {
	left := listenNode(t)
	right := listenNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := testNode("a", 0, left, "")
	b := testNode("b", 2, right, "")
	a.cfg.peers = gossip.Static{List: []gossip.Peer{{ID: "b", Addr: right.Addr()}}}
	b.cfg.peers = gossip.Static{List: []gossip.Peer{{ID: "a", Addr: left.Addr()}}}
	errc := make(chan error, 2)
	go func() { errc <- a.run(ctx) }()
	go func() { errc <- b.run(ctx) }()
	for range 2 {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}

	ap, an := a.snapshot()
	bp, bn := b.snapshot()
	if an == 0 || bn == 0 {
		t.Fatalf("neighbors a=%d b=%d", an, bn)
	}
	r, _ := core.Order([]core.Phase{ap, bp})
	if r < 0.95 {
		t.Fatalf("r = %v, phases %v %v", r, ap, bp)
	}
}

func TestSendErrorDoesNotStop(t *testing.T) {
	n := testNode("a", 0.3, idleTransport{sendErr: errors.New("drop")}, "")
	n.cfg.ticks = 4
	n.cfg.wait = 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := n.run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFirePostsToSink(t *testing.T) {
	var posts atomic.Int32
	var bad atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if closeErr := r.Body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			bad.Store(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var got struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &got); err != nil || got.ID != "a" {
			bad.Store(fmt.Sprintf("body %s", body))
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		posts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := testNode("a", twoPi-0.01, idleTransport{}, srv.URL)
	n.cfg.ticks = 3
	n.cfg.wait = 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := n.run(ctx); err != nil {
		t.Fatal(err)
	}
	if posts.Load() < 1 {
		t.Fatalf("posts = %d, want at least one", posts.Load())
	}
	if v := bad.Load(); v != nil {
		t.Fatal(v)
	}
}

func TestEmptySinkOnlyLogs(t *testing.T) {
	n := testNode("a", twoPi-0.01, idleTransport{}, "")
	n.cfg.ticks = 3
	n.cfg.wait = 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := n.run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFired(t *testing.T) {
	const (
		period = 1.0
		dt     = 0.01
	)
	omega := twoPi / period
	step := core.NewKuramoto(period, 0)

	prev := core.Wrap(twoPi - 0.01)
	next := step.Step(prev, nil, dt)
	if !fired(prev, next, omega, dt) {
		t.Fatal("expected a crossing")
	}
	prev = 0.4
	next = step.Step(prev, nil, dt)
	if fired(prev, next, omega, dt) {
		t.Fatal("did not expect a crossing")
	}
	if fired(prev, next, omega, 0) {
		t.Fatal("dt 0 must not fire")
	}
}

func TestParsePeers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.txt")
	body := []byte("# comment\n\nb 127.0.0.1:9002\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parsePeers("c=127.0.0.1:9003", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 2 || got.List[0].ID != "b" || got.List[1].Addr != "127.0.0.1:9003" {
		t.Fatalf("peers = %+v", got.List)
	}
	if _, err := parsePeers("nope", ""); err == nil {
		t.Fatal("expected an error for a peer without an address")
	}
}

func TestDefaultKuramotoK(t *testing.T) {
	if got := defaultKuramotoK("kuramoto", 0, false); got != 2 {
		t.Fatalf("omitted K = %v, want 2", got)
	}
	if got := defaultKuramotoK("sync", 0, false); got != 2 {
		t.Fatalf("sync K = %v, want 2", got)
	}
	if got := defaultKuramotoK("kuramoto", 0, true); got != 0 {
		t.Fatalf("explicit 0 = %v, want 0", got)
	}
	if got := defaultKuramotoK("desync", 0, false); got != 0 {
		t.Fatalf("desync K = %v, want 0", got)
	}
}

func testNode(id string, phase float64, tr gossip.Transport, sink string) *node {
	const period = 1.0
	return &node{
		cfg: config{
			id:      id,
			stepper: core.NewKuramoto(period, 2),
			omega:   twoPi / period,
			dt:      0.01,
			phase:   core.Wrap(phase),
			sink:    sink,
			peers:   gossip.Static{},
			ticks:   1000,
			wait:    time.Millisecond,
			log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		tr: tr,
	}
}

func (n *node) snapshot() (core.Phase, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.phase, len(n.heard)
}

func listenNode(t *testing.T) *gossip.UDP {
	t.Helper()
	u, err := gossip.ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return u
}

type idleTransport struct {
	sendErr error
}

func (t idleTransport) Send(context.Context, gossip.Peer, gossip.Message) error {
	return t.sendErr
}

func (t idleTransport) Receive(ctx context.Context) (gossip.Peer, gossip.Message, error) {
	<-ctx.Done()
	return gossip.Peer{}, gossip.Message{}, ctx.Err()
}
