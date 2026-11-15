// Command sweep runs the jitter / hash / DESYNC grid and the 30% kill.
// Python in experiments/plot.py turns the CSV into the table and the figures.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Starovoitov/phasefleet/internal/sim"
)

type row struct {
	strategy  string
	n, slots  int
	d, period float64
	seed      int64
	trial     int
	peak      int
	median    float64
	ideal     int
	k, alpha  float64
	steps     int
	settled   bool
	mad       float64
}

func main() {
	out := flag.String("out", "experiments/out", "directory for CSV")
	trials := flag.Int("trials", 5000, "jitter trials per grid row")
	maxN := flag.Int("max-n", 0, "skip rows with more nodes (0 runs the full grid)")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	grid := []struct{ n, slots int }{
		{20, 3600},
		{60, 60},
		{100, 60},
		{1000, 60},
		{10000, 60},
	}
	var rows []row
	for _, g := range grid {
		if *maxN > 0 && g.n > *maxN {
			continue
		}
		d, period := 1.0, float64(g.slots)
		fmt.Fprintf(os.Stderr, "row N=%d slots=%d\n", g.n, g.slots)
		rows = append(rows, steadyRows(sim.StrategyAligned, g.n, g.slots, d, period, 1, 1)...)
		rows = append(rows, steadyRows(sim.StrategyHash, g.n, g.slots, d, period, 1, 1)...)
		rows = append(rows, desyncRow(g.n, g.slots, d, period))
		rows = append(rows, steadyRows(sim.StrategyJitter, g.n, g.slots, d, period, 1, *trials)...)
	}
	if *maxN == 0 || *maxN >= 60 {
		fmt.Fprintln(os.Stderr, "kuramoto K=-1 at N=60")
		rows = append(rows, negativeK(60, 60))
	}
	if err := writeTrials(filepath.Join(*out, "trials.csv"), rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "recovery after killing 30%")
	ticks, ok, jitterMAD, series, err := sim.Recovery(60, 1, 0.5, 0.3, 50000, 1e-3)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "desync recovery ticks=%d settled=%v jitter MAD=%g\n", ticks, ok, jitterMAD)
	if err := writeRecovery(filepath.Join(*out, "recovery.csv"), series); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func steadyRows(strategy string, n, slots int, d, period float64, seed0 int64, trials int) []row {
	start := time.Now()
	f, err := sim.New(sim.Config{
		N: n, D: d, T: period,
		Strategy: strategy, Seed: seed0, KillAt: -1,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ideal := sim.IdealPeak(n, d, period)
	rows := make([]row, 0, trials)
	for trial := 0; trial < trials; trial++ {
		seed := seed0 + int64(trial)
		f.Reseed(seed)
		peak, median := f.SteadyPeak()
		rows = append(rows, row{
			strategy: strategy, n: n, slots: slots, d: d, period: period,
			seed: seed, trial: trial, peak: peak, median: median, ideal: ideal,
		})
	}
	fmt.Fprintf(os.Stderr, "  %s trials=%d %s\n", strategy, trials, time.Since(start).Round(time.Millisecond))
	return rows
}

func desyncRow(n, slots int, d, period float64) row {
	start := time.Now()
	const (
		alpha  = 0.5
		relTol = 1e-3
	)
	// Neighbor-only mixing clears the last extra overlap in O(N²) ticks.
	// N≤1000 reaches the ideal peak in this budget. N=10000 does not:
	// the row keeps the peak actually reached, and the table says so.
	max := 200000
	if n > 1000 {
		max = 30000
	}
	f, err := sim.New(sim.Config{
		N: n, D: d, T: period,
		Strategy: sim.StrategyDesync, Alpha: alpha, Seed: 1, KillAt: -1,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ideal := sim.IdealPeak(n, d, period)
	f.SetDt(0)
	steps := 0
	ok := false
	peak, median := 0, 0.0
	for steps = 1; steps <= max; steps++ {
		f.Tick()
		if steps%50 != 0 && steps != max && !f.Uniform(relTol) {
			continue
		}
		peak, median = f.SteadyPeak()
		if steps%2000 == 0 {
			fmt.Fprintf(os.Stderr, "    desync N=%d step=%d peak=%d ideal=%d\n", n, steps, peak, ideal)
		}
		if peak == ideal {
			ok = true
			break
		}
	}
	if steps > max {
		steps = max
		peak, median = f.SteadyPeak()
		ok = peak == ideal
	}
	fmt.Fprintf(os.Stderr, "  desync steps=%d settled=%v MAD=%g peak=%d ideal=%d %s\n",
		steps, ok, f.GapMAD(), peak, ideal, time.Since(start).Round(time.Millisecond))
	return row{
		strategy: sim.StrategyDesync, n: n, slots: slots, d: d, period: period,
		seed: 1, peak: peak, median: median, ideal: ideal,
		alpha: alpha, steps: steps, settled: ok, mad: f.GapMAD(),
	}
}

func negativeK(n, slots int) row {
	const dt = 0.05
	d, period := 1.0, float64(slots)
	start := time.Now()
	f, err := sim.New(sim.Config{
		N: n, D: d, T: period, Dt: dt,
		Strategy: sim.StrategyKuramoto, K: -1, Seed: 1, KillAt: -1,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	warm := int(20 * period / dt)
	for i := 0; i < warm; i++ {
		f.Tick()
	}
	measure := int(period / dt)
	var fires []sim.Fire
	for i := 0; i < measure; i++ {
		fires = append(fires, f.Tick()...)
	}
	arrivals := make([]float64, len(fires))
	for i, fire := range fires {
		arrivals[i] = fire.Arrival
	}
	r, _ := f.Order()
	peak := sim.PeakOverlap(arrivals, d)
	fmt.Fprintf(os.Stderr, "  kuramoto K=-1 peak=%d r=%g %s\n", peak, r, time.Since(start).Round(time.Millisecond))
	return row{
		strategy: sim.StrategyKuramoto, n: n, slots: slots, d: d, period: period,
		seed: 1, peak: peak, median: sim.MedianInterval(arrivals),
		ideal: sim.IdealPeak(n, d, period), k: -1, settled: true, mad: r,
	}
}

func writeTrials(path string, rows []row) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w := csv.NewWriter(f)
	header := []string{
		"strategy", "N", "slots", "d", "T", "seed", "trial", "peak", "median_interval",
		"ideal", "K", "alpha", "steps", "settled", "gap_mad",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			r.strategy,
			strconv.Itoa(r.n),
			strconv.Itoa(r.slots),
			strconv.FormatFloat(r.d, 'g', -1, 64),
			strconv.FormatFloat(r.period, 'g', -1, 64),
			strconv.FormatInt(r.seed, 10),
			strconv.Itoa(r.trial),
			strconv.Itoa(r.peak),
			strconv.FormatFloat(r.median, 'g', -1, 64),
			strconv.Itoa(r.ideal),
			strconv.FormatFloat(r.k, 'g', -1, 64),
			strconv.FormatFloat(r.alpha, 'g', -1, 64),
			strconv.Itoa(r.steps),
			strconv.FormatBool(r.settled),
			strconv.FormatFloat(r.mad, 'g', -1, 64),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeRecovery(path string, series []sim.Point) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"strategy", "tick", "gap_mad", "alive"}); err != nil {
		return err
	}
	for _, p := range series {
		err := w.Write([]string{
			p.Strategy,
			strconv.Itoa(p.Tick),
			strconv.FormatFloat(p.MAD, 'g', -1, 64),
			strconv.Itoa(p.Alive),
		})
		if err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
