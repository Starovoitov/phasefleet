package gossip

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// srvLookup is the slice of net.Resolver that DNS needs.
// A headless Service publishes SRV records named _port._proto.service.
type srvLookup interface {
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// DNS is a PeerSource for a Kubernetes headless Service.
// Name is the service FQDN. Peers come from SRV _gossip._udp.<name>,
// so the Service port must be named gossip and use UDP.
// The peer id is the first label of the pod DNS name, which on a
// StatefulSet is the pod name.
type DNS struct {
	Name    string
	Service string
	Proto   string
	TTL     time.Duration

	resolver srvLookup

	mu     sync.Mutex
	cached []Peer
	until  time.Time
}

// NewDNS looks up the headless Service name with the process resolver.
// Results are reused for a second so a tick does not query DNS every step.
func NewDNS(name string) *DNS {
	return &DNS{
		Name:     name,
		Service:  "gossip",
		Proto:    "udp",
		TTL:      time.Second,
		resolver: net.DefaultResolver,
	}
}

// Peers returns the current pod addresses. A not-found answer is an empty
// fleet, not a failure. A later error keeps the previous answer.
func (d *DNS) Peers(ctx context.Context) ([]Peer, error) {
	if d.TTL > 0 {
		d.mu.Lock()
		if len(d.cached) > 0 && time.Now().Before(d.until) {
			out := append([]Peer(nil), d.cached...)
			d.mu.Unlock()
			return out, nil
		}
		d.mu.Unlock()
	}

	list, err := d.lookup(ctx)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return []Peer{}, nil
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		if len(d.cached) > 0 {
			return append([]Peer(nil), d.cached...), nil
		}
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.cached = append([]Peer(nil), list...)
	if d.TTL > 0 {
		d.until = time.Now().Add(d.TTL)
	}
	return append([]Peer(nil), list...), nil
}

func (d *DNS) lookup(ctx context.Context) ([]Peer, error) {
	resolver := d.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	service := d.Service
	if service == "" {
		service = "gossip"
	}
	proto := d.Proto
	if proto == "" {
		proto = "udp"
	}
	_, records, err := resolver.LookupSRV(ctx, service, proto, d.Name)
	if err != nil {
		return nil, err
	}
	out := make([]Peer, 0, len(records))
	for _, rec := range records {
		host := strings.TrimSuffix(rec.Target, ".")
		if host == "" {
			continue
		}
		port := rec.Port
		if port == 0 {
			port = 9000
		}
		id := host
		if label, _, ok := strings.Cut(host, "."); ok {
			id = label
		}
		out = append(out, Peer{ID: id, Addr: net.JoinHostPort(host, strconv.Itoa(int(port)))})
	}
	return out, nil
}

// Merge is a PeerSource that concatenates several sources.
// Static lists and a DNS headless name can both be set.
type Merge []PeerSource

// Peers appends every source. The first error stops the merge.
func (m Merge) Peers(ctx context.Context) ([]Peer, error) {
	var out []Peer
	for _, src := range m {
		list, err := src.Peers(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, list...)
	}
	return out, nil
}
