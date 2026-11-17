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

	"github.com/Starovoitov/phasefleet/internal/clienv"
	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/gossip"
)

func main() {
	kDefault, kFromEnv, err := clienv.Float("PHASEFLEET_K")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	alphaDefault, alphaSet, err := clienv.Float("PHASEFLEET_ALPHA")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	if !alphaSet {
		alphaDefault = 0.5
	}
	periodDefault, periodSet, err := clienv.Float("PHASEFLEET_T")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	if !periodSet {
		periodDefault = 1
	}
	dtDefault, dtSet, err := clienv.Float("PHASEFLEET_DT")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	if !dtSet {
		dtDefault = -1
	}
	phaseDefault := math.NaN()
	if v, ok, err := clienv.Float("PHASEFLEET_PHASE"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	} else if ok {
		phaseDefault = v
	}
	ticksDefault := 0
	if v, ok, err := clienv.Int("PHASEFLEET_TICKS"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	} else if ok {
		ticksDefault = v
	}

	id := flag.String("id", clienv.String("PHASEFLEET_ID", ""), "member id")
	listen := flag.String("listen", clienv.String("PHASEFLEET_LISTEN", ":9000"), "UDP listen address")
	peerList := flag.String("peers", clienv.String("PHASEFLEET_PEERS", ""), "comma-separated id=host:port")
	peerFile := flag.String("peers-file", clienv.String("PHASEFLEET_PEERS_FILE", ""), "file of peers, one id=host:port or \"id host:port\" per line")
	strategy := flag.String("strategy", clienv.String("PHASEFLEET_STRATEGY", "kuramoto"), "kuramoto or desync")
	k := flag.Float64("K", kDefault, "Kuramoto coupling; omitted on kuramoto means 2")
	alpha := flag.Float64("alpha", alphaDefault, "DESYNC fraction of the centering move")
	period := flag.Float64("T", periodDefault, "period of this member")
	dt := flag.Float64("dt", dtDefault, "tick length; default T/100")
	phase := flag.Float64("phase", phaseDefault, "initial phase in radians; default is a hash of -id")
	sink := flag.String("sink", clienv.String("PHASEFLEET_SINK", ""), "sink base URL; empty logs the firing only")
	metricsAddr := flag.String("metrics", clienv.Optional("PHASEFLEET_METRICS", ":9100"), "HTTP address for /metrics; empty disables it")
	ticks := flag.Int("ticks", ticksDefault, "steps to run; 0 runs until interrupted")
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
	kSet := kFromEnv
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
