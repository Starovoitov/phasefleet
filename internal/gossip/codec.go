package gossip

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
)

// packet is the UDP body. sent_at is stored and not applied: delay
// compensation belongs to a later geo run, ahead of the step.
type packet struct {
	ID     string  `json:"id"`
	Phase  float64 `json:"phase"`
	Seq    uint64  `json:"seq"`
	SentAt int64   `json:"sent_at"`
}

func encodeMessage(msg Message) ([]byte, error) {
	body, err := json.Marshal(packet{
		ID:     msg.ID,
		Phase:  float64(msg.Phase),
		Seq:    msg.Seq,
		SentAt: msg.SentAt.UnixNano(),
	})
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}
	return body, nil
}

func decodeMessage(body []byte) (Message, error) {
	var pkt packet
	if err := json.Unmarshal(body, &pkt); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	if pkt.ID == "" {
		return Message{}, fmt.Errorf("decode message: empty id")
	}
	return Message{
		ID:     pkt.ID,
		Phase:  core.Wrap(pkt.Phase),
		Seq:    pkt.Seq,
		SentAt: time.Unix(0, pkt.SentAt),
	}, nil
}
