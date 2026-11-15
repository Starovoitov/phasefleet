package gossip

import "context"

// Transport moves phase messages.
// The first implementation is UDP. Losses and reordering are normal.
type Transport interface {
	Send(ctx context.Context, to Peer, msg Message) error
	Receive(ctx context.Context) (from Peer, msg Message, err error)
}
