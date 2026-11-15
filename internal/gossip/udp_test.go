package gossip

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
)

func TestUDPRoundTrip(t *testing.T) {
	a := listenUDP(t)
	b := listenUDP(t)

	sentAt := time.Unix(0, 1_700_000_000_000_000_000)
	want := Message{ID: "a", Phase: core.Wrap(1.25), Seq: 7, SentAt: sentAt}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := a.Send(ctx, Peer{ID: "b", Addr: b.Addr()}, want); err != nil {
		t.Fatal(err)
	}
	from, got, err := b.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if from.ID != want.ID || from.Addr == "" {
		t.Fatalf("from = %+v", from)
	}
	if got.ID != want.ID || got.Seq != want.Seq || got.SentAt.UnixNano() != sentAt.UnixNano() {
		t.Fatalf("message = %+v", got)
	}
	if got.Phase != want.Phase {
		t.Fatalf("phase = %v, want %v", got.Phase, want.Phase)
	}
}

func TestUDPDropsOlderSeq(t *testing.T) {
	a := listenUDP(t)
	b := listenUDP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	send := func(seq uint64) {
		t.Helper()
		msg := Message{ID: "a", Phase: 0.5, Seq: seq, SentAt: time.Unix(0, int64(seq))}
		if err := a.Send(ctx, Peer{ID: "b", Addr: b.Addr()}, msg); err != nil {
			t.Fatal(err)
		}
	}
	recv := func() uint64 {
		t.Helper()
		_, msg, err := b.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return msg.Seq
	}

	send(1)
	if got := recv(); got != 1 {
		t.Fatalf("seq = %d, want 1", got)
	}
	// A gap is a loss, not a stall: 3 is delivered, then a late 2 is dropped.
	send(3)
	if got := recv(); got != 3 {
		t.Fatalf("seq = %d, want 3", got)
	}
	send(2)
	send(4)
	if got := recv(); got != 4 {
		t.Fatalf("seq = %d, want 4 (2 is older than 3)", got)
	}
}

func TestUDPSkipsGarbage(t *testing.T) {
	a := listenUDP(t)
	b := listenUDP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dst, err := net.ResolveUDPAddr("udp", b.Addr())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.conn.WriteToUDP([]byte("not json"), dst); err != nil {
		t.Fatal(err)
	}
	msg := Message{ID: "a", Phase: 1, Seq: 1, SentAt: time.Unix(0, 1)}
	if err := a.Send(ctx, Peer{Addr: b.Addr()}, msg); err != nil {
		t.Fatal(err)
	}
	_, got, err := b.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 1 || got.ID != "a" {
		t.Fatalf("message = %+v", got)
	}
}

func listenUDP(t *testing.T) *UDP {
	t.Helper()
	u, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return u
}
