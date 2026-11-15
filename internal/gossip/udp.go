package gossip

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// UDP is a Transport over one datagram socket.
// Receive drops a packet whose seq is older than the last seq accepted
// from that id, then keeps reading. A gap in the sequence is not an error:
// the caller simply does not see the lost packet.
type UDP struct {
	conn *net.UDPConn

	mu   sync.Mutex
	last map[string]uint64
	seen map[string]bool
}

// ListenUDP binds a UDP socket. addr may be host:port or :port; port 0
// asks the kernel for a free port.
func ListenUDP(addr string) (*UDP, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		return nil, err
	}
	return &UDP{
		conn: conn,
		last: map[string]uint64{},
		seen: map[string]bool{},
	}, nil
}

// Addr is the bound address, including the port the kernel chose.
func (u *UDP) Addr() string {
	return u.conn.LocalAddr().String()
}

// Close unblocks Receive.
func (u *UDP) Close() error {
	return u.conn.Close()
}

// Send writes one message to peer.Addr. The peer id is not on the wire
// separately: it is already inside msg.
func (u *UDP) Send(ctx context.Context, to Peer, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dst, err := net.ResolveUDPAddr("udp", to.Addr)
	if err != nil {
		return err
	}
	body, err := encodeMessage(msg)
	if err != nil {
		return err
	}
	_, err = u.conn.WriteToUDP(body, dst)
	return err
}

// Receive returns the next message that is not older than one already
// accepted from the same id. Malformed datagrams are skipped.
func (u *UDP) Receive(ctx context.Context) (Peer, Message, error) {
	buf := make([]byte, 2048)
	for {
		if err := ctx.Err(); err != nil {
			return Peer{}, Message{}, err
		}
		if err := u.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
			return Peer{}, Message{}, err
		}
		n, addr, err := u.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return Peer{}, Message{}, err
		}
		msg, err := decodeMessage(buf[:n])
		if err != nil {
			continue
		}
		if u.stale(msg) {
			continue
		}
		return Peer{ID: msg.ID, Addr: addr.String()}, msg, nil
	}
}

// stale reports whether msg.Seq is strictly older than the last accepted
// seq from msg.ID, and records msg when it is not.
func (u *UDP) stale(msg Message) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.seen[msg.ID] && msg.Seq < u.last[msg.ID] {
		return true
	}
	u.seen[msg.ID] = true
	u.last[msg.ID] = msg.Seq
	return false
}

// Compile-time check that UDP is a Transport.
var _ Transport = (*UDP)(nil)
