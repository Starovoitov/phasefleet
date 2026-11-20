package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/gossip"
	"github.com/Starovoitov/phasefleet/internal/sink"
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

func TestFireRaisesSinkPeak(t *testing.T) {
	record := sink.New(time.Second)
	srv := httptest.NewServer(sink.Handler(record))
	defer srv.Close()

	n := testNode("a", twoPi-0.01, idleTransport{}, srv.URL)
	n.cfg.ticks = 3
	n.cfg.wait = 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := n.run(ctx); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if record.Peak() < 1 {
		t.Fatalf("peak = %d, want at least 1", record.Peak())
	}
	if got := scrapeGauge(t, string(body), "phasefleet_sink_peak"); got != float64(record.Peak()) {
		t.Fatalf("phasefleet_sink_peak = %v, want %d", got, record.Peak())
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

func TestFireURL(t *testing.T) {
	got, err := fireURL("http://127.0.0.1:8080")
	if err != nil || got != "http://127.0.0.1:8080/fire" {
		t.Fatalf("base = %q, %v", got, err)
	}
	got, err = fireURL("http://127.0.0.1:8080/")
	if err != nil || got != "http://127.0.0.1:8080/fire" {
		t.Fatalf("slash = %q, %v", got, err)
	}
	got, err = fireURL("http://127.0.0.1:8080/fire")
	if err != nil || got != "http://127.0.0.1:8080/fire" {
		t.Fatalf("path = %q, %v", got, err)
	}
}

func TestPhaseOffsetRemovesRotation(t *testing.T) {
	const omega = twoPi
	elapsed := 1500 * time.Millisecond
	phase := core.Wrap(1 + omega*elapsed.Seconds())
	got := phaseOffset(phase, omega, elapsed)
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("offset = %v, want 1", got)
	}
}

func TestPhaseMetrics(t *testing.T) {
	m := newPhaseMetrics()
	m.set(1.25)
	srv := httptest.NewServer(m.handler)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "phasefleet_node_phase_offset_radians 1.25") {
		t.Fatalf("metrics:\n%s", body)
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

func TestPeerSourceJoinsStaticAndDNS(t *testing.T) {
	src, n, err := peerSource("b=127.0.0.1:9002", "", "node.phasefleet.svc.cluster.local")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("static count = %d", n)
	}
	if _, ok := src.(gossip.Merge); !ok {
		t.Fatalf("source = %T, want gossip.Merge", src)
	}
	only, _, err := peerSource("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	static, ok := only.(gossip.Static)
	if !ok || len(static.List) != 0 {
		t.Fatalf("empty source = %#v", only)
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

func scrapeGauge(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name+" ") {
			continue
		}
		fields := strings.Fields(line)
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Fatalf("metric %s not found", name)
	return 0
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
