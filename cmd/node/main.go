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
	peersDNS := flag.String("peers-dns", clienv.String("PHASEFLEET_PEERS_DNS", ""), "headless Service FQDN; peers come from SRV _gossip._udp")
	strategy := flag.String("strategy", clienv.String("PHASEFLEET_STRATEGY", "kuramoto"), "kuramoto or desync")
	aggregate := flag.String("aggregate", clienv.String("PHASEFLEET_AGGREGATE", "mean"), "mean or median; median is kuramoto only")
	lock := flag.Bool("lock", envLock("PHASEFLEET_LOCK"), "publish -phase forever and ignore neighbors")
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
	stepper, err := newStepper(name, *aggregate, *period, coupling, *alpha)
	if err != nil {
		slog.Error("strategy", "err", err)
		os.Exit(2)
	}
	peers, staticN, err := peerSource(*peerList, *peerFile, *peersDNS)
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
		"aggregate", *aggregate,
		"lock", *lock,
		"peers", staticN,
		"peers_dns", *peersDNS,
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
			lock:    *lock,
			onPhase: onPhase,
		},
		tr: tr,
	}
	if err := member.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("node", "err", err)
		os.Exit(1)
	}
}

func newStepper(strategy, aggregate string, period, k, alpha float64) (core.Stepper, error) {
	switch aggregate {
	case "mean", "median":
	default:
		return nil, fmt.Errorf("unknown aggregate %q", aggregate)
	}
	if aggregate == "median" && strategy != "kuramoto" && strategy != "sync" {
		return nil, fmt.Errorf("median aggregate is only defined for kuramoto")
	}
	switch strategy {
	case "kuramoto", "sync":
		if aggregate == "median" {
			return core.NewMedianKuramoto(period, k), nil
		}
		return core.NewKuramoto(period, k), nil
	case "desync":
		return core.NewDesync(period, alpha), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", strategy)
	}
}

// envLock is true for 1, true, and yes. A blank variable stays false.
func envLock(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// defaultKuramotoK turns an omitted -K into attraction. An explicit -K, including 0, is kept.
func defaultKuramotoK(name string, k float64, kSet bool) float64 {
	if (name == "kuramoto" || name == "sync") && !kSet {
		return 2
	}
	return k
}

func peerSource(list, file, dnsName string) (gossip.PeerSource, int, error) {
	static, err := parsePeers(list, file)
	if err != nil {
		return nil, 0, err
	}
	var sources []gossip.PeerSource
	if len(static.List) > 0 {
		sources = append(sources, static)
	}
	if dnsName != "" {
		sources = append(sources, gossip.NewDNS(dnsName))
	}
	switch len(sources) {
	case 0:
		return gossip.Static{}, 0, nil
	case 1:
		return sources[0], len(static.List), nil
	default:
		return gossip.Merge(sources), len(static.List), nil
	}
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
