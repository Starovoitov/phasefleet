package gossip

import (
	"context"
	"testing"
)

func TestStaticPeers(t *testing.T) {
	src := Static{List: []Peer{{ID: "a", Addr: "127.0.0.1:9000"}}}
	got, err := src.Peers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("Peers = %+v", got)
	}
}
