package gossip

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type scriptSRV struct {
	calls int
	addrs []*net.SRV
	err   error
}

func (s *scriptSRV) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	s.calls++
	if s.err != nil {
		return "", nil, s.err
	}
	return "", s.addrs, nil
}

func TestDNSPeersFromSRV(t *testing.T) {
	src := NewDNS("node.phasefleet.svc.cluster.local")
	src.TTL = 0
	script := &scriptSRV{addrs: []*net.SRV{
		{Target: "node-1.node.phasefleet.svc.cluster.local.", Port: 9000},
		{Target: "node-0.node.phasefleet.svc.cluster.local.", Port: 9000},
	}}
	src.resolver = script

	got, err := src.Peers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Peers = %+v", got)
	}
	if got[0].ID != "node-1" || got[0].Addr != "node-1.node.phasefleet.svc.cluster.local:9000" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].ID != "node-0" || got[1].Addr != "node-0.node.phasefleet.svc.cluster.local:9000" {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestDNSCaches(t *testing.T) {
	src := NewDNS("node.phasefleet.svc.cluster.local")
	src.TTL = time.Minute
	script := &scriptSRV{addrs: []*net.SRV{
		{Target: "node-0.node.phasefleet.svc.cluster.local.", Port: 9000},
	}}
	src.resolver = script

	if _, err := src.Peers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Peers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if script.calls != 1 {
		t.Fatalf("lookups = %d, want 1 while the cache is fresh", script.calls)
	}
}

func TestDNSKeepsLastAnswer(t *testing.T) {
	src := NewDNS("node.phasefleet.svc.cluster.local")
	src.TTL = 0
	script := &scriptSRV{addrs: []*net.SRV{
		{Target: "node-0.node.phasefleet.svc.cluster.local.", Port: 9000},
	}}
	src.resolver = script
	if _, err := src.Peers(context.Background()); err != nil {
		t.Fatal(err)
	}
	script.err = errors.New("temporary")
	got, err := src.Peers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "node-0" {
		t.Fatalf("Peers = %+v", got)
	}
}

func TestDNSNotFoundIsEmpty(t *testing.T) {
	src := NewDNS("node.phasefleet.svc.cluster.local")
	src.TTL = 0
	src.resolver = &scriptSRV{err: &net.DNSError{Err: "no such host", Name: src.Name, IsNotFound: true}}
	got, err := src.Peers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Peers = %+v, want none", got)
	}
}

func TestMergePeers(t *testing.T) {
	got, err := Merge{
		Static{List: []Peer{{ID: "a", Addr: "a:9000"}}},
		Static{List: []Peer{{ID: "b", Addr: "b:9000"}}},
	}.Peers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("Peers = %+v", got)
	}
}
