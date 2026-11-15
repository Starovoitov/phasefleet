package gossip

import "context"

// Peer is a fleet member the local node can address.
type Peer struct {
	ID   string
	Addr string
}

// PeerSource lists the peers visible right now.
// In-memory, DNS from a headless Service, a seed list, and memberlist
// are interchangeable behind this.
type PeerSource interface {
	Peers(ctx context.Context) ([]Peer, error)
}

// Static is a PeerSource with a fixed list. The simulator and tests use it.
type Static struct {
	List []Peer
}

// Peers returns the fixed list. ctx is unused.
func (s Static) Peers(context.Context) ([]Peer, error) {
	return s.List, nil
}
