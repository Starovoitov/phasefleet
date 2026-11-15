// Command node is one fleet member. It gossips its phase over UDP and
// steps with the same Stepper as the simulator.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/gossip"
)

func main() {
	id := flag.String("id", "", "member id")
	listen := flag.String("listen", ":9000", "UDP listen address")
	peerList := flag.String("peers", "", "comma-separated id=host:port")
	peerFile := flag.String("peers-file", "", "file of peers, one id=host:port or \"id host:port\" per line")
	strategy := flag.String("strategy", "kuramoto", "kuramoto or desync")
	k := flag.Float64("K", 0, "Kuramoto coupling; omitted on kuramoto means 2")
	alpha := flag.Float64("alpha", 0.5, "DESYNC fraction of the centering move")
	period := flag.Float64("T", 1, "period of this member")
	dt := flag.Float64("dt", -1, "tick length; default T/100")
	phase := flag.Float64("phase", math.NaN(), "initial phase in radians; default is a hash of -id")
	sink := flag.String("sink", "", "sink base URL; empty logs the firing only")
	metricsAddr := flag.String("metrics", ":9100", "HTTP address for /metrics; empty disables it")
	ticks := flag.Int("ticks", 0, "steps to run; 0 runs until interrupted")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if *id == "" {
		slog.Error("id is required")
		os.Exit(2)
	}
	if *period <= 0 {
		slog.Error("period must be positive")
		os.Exit(2)
	}
	step := *dt
	if step < 0 {
		step = *period / 100
	}
	if step <= 0 {
		slog.Error("dt must be positive")
		os.Exit(2)
	}
	kSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "K" {
			kSet = true
		}
	})
	name := *strategy
	coupling := defaultKuramotoK(name, *k, kSet)
	stepper, err := newStepper(name, *period, coupling, *alpha)
	if err != nil {
		slog.Error("strategy", "err", err)
		os.Exit(2)
	}
	peers, err := parsePeers(*peerList, *peerFile)
	if err != nil {
		slog.Error("peers", "err", err)
		os.Exit(2)
	}
	initial := *phase
	if math.IsNaN(initial) {
		initial = float64(phaseOfID(*id))
	}

	tr, err := gossip.ListenUDP(*listen)
	if err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := tr.Close(); err != nil {
			slog.Error("close", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var onPhase func(float64)
	if *metricsAddr != "" {
		metrics := newPhaseMetrics()
		onPhase = metrics.set
		go func() {
			if err := serveMetrics(ctx, *metricsAddr, metrics.handler); err != nil {
				slog.Error("metrics", "err", err)
				stop()
			}
		}()
	}

	slog.Info("phasefleet node",
		"id", *id,
		"listen", tr.Addr(),
		"strategy", name,
		"peers", len(peers.List),
		"sink", *sink,
		"metrics", *metricsAddr,
	)
	member := &node{
		cfg: config{
			id:      *id,
			stepper: stepper,
			omega:   twoPi / *period,
			dt:      step,
			phase:   core.Wrap(initial),
			sink:    *sink,
			peers:   peers,
			ticks:   *ticks,
			wait:    time.Duration(step * float64(time.Second)),
			onPhase: onPhase,
		},
		tr: tr,
	}
	if err := member.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("node", "err", err)
		os.Exit(1)
	}
}

func newStepper(strategy string, period, k, alpha float64) (core.Stepper, error) {
	switch strategy {
	case "kuramoto", "sync":
		return core.NewKuramoto(period, k), nil
	case "desync":
		return core.NewDesync(period, alpha), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", strategy)
	}
}

// defaultKuramotoK turns an omitted -K into attraction. An explicit -K, including 0, is kept.
func defaultKuramotoK(name string, k float64, kSet bool) float64 {
	if (name == "kuramoto" || name == "sync") && !kSet {
		return 2
	}
	return k
}

func parsePeers(list, file string) (gossip.Static, error) {
	var out []gossip.Peer
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return gossip.Static{}, err
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			peer, err := parsePeer(line)
			if err != nil {
				return gossip.Static{}, err
			}
			out = append(out, peer)
		}
	}
	if list != "" {
		for _, part := range strings.Split(list, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			peer, err := parsePeer(part)
			if err != nil {
				return gossip.Static{}, err
			}
			out = append(out, peer)
		}
	}
	return gossip.Static{List: out}, nil
}

func parsePeer(s string) (gossip.Peer, error) {
	if id, addr, ok := strings.Cut(s, "="); ok {
		id = strings.TrimSpace(id)
		addr = strings.TrimSpace(addr)
		if id == "" || addr == "" {
			return gossip.Peer{}, fmt.Errorf("peer %q must be id=host:port", s)
		}
		return gossip.Peer{ID: id, Addr: addr}, nil
	}
	fields := strings.Fields(s)
	if len(fields) == 2 {
		return gossip.Peer{ID: fields[0], Addr: fields[1]}, nil
	}
	return gossip.Peer{}, fmt.Errorf("peer %q must be id=host:port", s)
}
