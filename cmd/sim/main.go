// Command sim runs a fleet in one process and writes a firing trace.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Starovoitov/phasefleet/internal/sim"
)

func main() {
	n := flag.Int("n", 100, "nodes in the fleet")
	mode := flag.String("mode", "", "legacy name: sync, desync, or jitter")
	strategy := flag.String("strategy", "", "aligned, jitter, hash, kuramoto, or desync")
	ticks := flag.Int("ticks", 1000, "simulation steps")
	d := flag.Float64("d", 0.1, "task duration")
	period := flag.Float64("T", 1, "period of each node")
	dt := flag.Float64("dt", -1, "tick length; default T/100")
	seed := flag.Int64("seed", 1, "jitter and desync seed")
	k := flag.Float64("K", 0, "Kuramoto coupling")
	alpha := flag.Float64("alpha", 0.5, "DESYNC fraction of the centering move")
	csvPath := flag.String("csv", "sim.csv", "trace path")
	killAt := flag.Int("kill-at", -1, "tick at which to drop a fraction of nodes")
	kill := flag.Float64("kill", 0, "fraction killed at -kill-at")
	flag.Parse()

	name := *strategy
	if name == "" {
		name = mapMode(*mode)
	}
	step := *dt
	if step < 0 {
		step = *period / 100
	}
	kSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "K" {
			kSet = true
		}
	})
	coupling := defaultKuramotoK(name, *k, kSet)
	cfg := sim.Config{
		N:            *n,
		D:            *d,
		T:            *period,
		Dt:           step,
		Ticks:        *ticks,
		Seed:         *seed,
		Strategy:     name,
		K:            coupling,
		Alpha:        *alpha,
		KillAt:       *killAt,
		KillFraction: *kill,
		Trace:        true,
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	res, err := sim.Run(cfg)
	if err != nil {
		slog.Error("sim", "err", err)
		os.Exit(1)
	}
	out, err := os.Create(*csvPath)
	if err != nil {
		slog.Error("csv", "err", err)
		os.Exit(1)
	}
	err = sim.WriteCSV(out, cfg, res.Samples)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		slog.Error("csv", "err", err)
		os.Exit(1)
	}
	alive := 0
	for _, node := range res.Final {
		if node.Alive {
			alive++
		}
	}
	slog.Info("phasefleet sim",
		"strategy", name,
		"n", *n,
		"peak", res.Peak,
		"ideal", sim.IdealPeak(*n, *d, *period),
		"fires", len(res.Fires),
		"alive", alive,
		"csv", *csvPath,
	)
}

// defaultKuramotoK turns an omitted -K into attraction. An explicit -K, including 0, is kept.
func defaultKuramotoK(name string, k float64, kSet bool) float64 {
	if name == sim.StrategyKuramoto && !kSet {
		return 2
	}
	return k
}

func mapMode(mode string) string {
	switch mode {
	case "", "desync":
		return sim.StrategyDesync
	case "jitter":
		return sim.StrategyJitter
	case "sync", "kuramoto":
		return sim.StrategyKuramoto
	case "aligned", "hash":
		return mode
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", mode)
		os.Exit(2)
		return ""
	}
}
