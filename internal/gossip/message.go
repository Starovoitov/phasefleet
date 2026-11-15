// Package gossip carries phase messages between nodes.
// Packet loss is acceptable: the step in internal/core tolerates a missing neighbor.
package gossip

import (
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
)

// Message is one phase announcement.
// SentAt is the sender's wall clock at transmit time, so a later geo mode
// can compensate delay from the timestamp and a measured RTT.
type Message struct {
	ID     string
	Phase  core.Phase
	Seq    uint64
	SentAt time.Time
}
