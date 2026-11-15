package sim

import (
	"bytes"
	"encoding/csv"
	"math"
	"testing"

	"github.com/Starovoitov/phasefleet/internal/core"
)

func TestAlignedPeakIsTheWholeFleet(t *testing.T) {
	res, err := Run(Config{
		N: 8, D: 0.2, T: 1, Dt: 1, Ticks: 1,
		Strategy: StrategyAligned, KillAt: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Peak != 8 {
		t.Fatalf("peak = %d, want 8 (every node fires together)", res.Peak)
	}
}

func TestDesyncPeakMatchesIdeal(t *testing.T) {
	const (
		n      = 12
		period = 1.0
		d      = 0.2
	)
	f, err := New(Config{
		N: n, D: d, T: period,
		Strategy: StrategyDesync, Alpha: 0.5, Seed: 3, KillAt: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Relax(20000, 0.02); !ok {
		t.Fatalf("gaps did not settle, MAD %v", f.GapMAD())
	}
	got, _ := f.SteadyPeak()
	want := IdealPeak(n, d, period)
	if got != want {
		t.Fatalf("peak = %d, want ideal %d", got, want)
	}
}

func TestKillDropsDeadFromTrace(t *testing.T) {
	cfg := Config{
		N: 10, D: 0.2, T: 1, Dt: 1, Ticks: 4,
		Strategy: StrategyAligned, KillAt: 1, KillFraction: 0.3, Trace: true,
	}
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	alive := 0
	for _, n := range res.Final {
		if n.Alive {
			alive++
		}
	}
	if alive != 7 {
		t.Fatalf("alive = %d, want 7", alive)
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, cfg, res.Samples); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		tick := row[4]
		id := row[5]
		if tick == "0" {
			continue
		}
		seen[id] = true
		if id == "9" || id == "8" || id == "7" {
			t.Fatalf("dead id %s still in the trace after the kill", id)
		}
	}
	if len(seen) != 7 {
		t.Fatalf("ids after the kill = %d, want 7", len(seen))
	}
}

func TestDelayHoldsThePhaseBack(t *testing.T) {
	delay := [][]int{{0, 3}, {3, 0}}
	f, err := New(Config{
		N: 2, D: 0.1, T: 1, Dt: 0,
		Strategy: StrategyDesync, Alpha: 0.5,
		Delay: delay, KillAt: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.phase[0] = 0
	f.phase[1] = core.Phase(math.Pi / 2)
	copy(f.hist[0], f.phase)
	for i := 0; i < 3; i++ {
		f.Tick()
	}
	if f.phase[0] != 0 || f.phase[1] != core.Phase(math.Pi/2) {
		t.Fatalf("phases moved before the message arrived: %v %v", f.phase[0], f.phase[1])
	}
	f.Tick()
	if f.phase[0] == 0 && f.phase[1] == core.Phase(math.Pi/2) {
		t.Fatal("phases stayed put after the delayed message arrived")
	}
}

func TestHashIsStable(t *testing.T) {
	cfg := Config{N: 5, D: 0.1, T: 1, Strategy: StrategyHash, KillAt: -1}
	a, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Final {
		if a.Final[i].Phase != b.Final[i].Phase {
			t.Fatalf("id %s phase changed across runs", a.Final[i].ID)
		}
	}
	if a.Final[0].Phase == 0 && a.Final[1].Phase == 0 {
		t.Fatal("hash phases collapsed to zero")
	}
}

func TestSinkDelayShiftsArrival(t *testing.T) {
	res, err := Run(Config{
		N: 1, D: 0.2, T: 1, Dt: 1, Ticks: 1,
		Strategy: StrategyAligned, SinkDelay: []float64{0.5}, KillAt: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fires) != 1 {
		t.Fatalf("fires = %d, want 1", len(res.Fires))
	}
	if math.Abs(res.Fires[0].Send-1) > 1e-9 || math.Abs(res.Fires[0].Arrival-1.5) > 1e-9 {
		t.Fatalf("send %v arrival %v, want 1 and 1.5", res.Fires[0].Send, res.Fires[0].Arrival)
	}
}

func TestRecoveryAfterKill(t *testing.T) {
	ticks, ok, jitterMAD, series, err := Recovery(8, 2, 0.5, 0.25, 20000, 0.02)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("desync did not restore the gaps")
	}
	if ticks < 1 {
		t.Fatalf("recovery ticks = %d", ticks)
	}
	var restored float64
	for _, p := range series {
		if p.Strategy == StrategyDesync && p.Tick == ticks {
			restored = p.MAD
		}
	}
	if jitterMAD <= restored*2 {
		t.Fatalf("jitter MAD %v, restored DESYNC MAD %v: the hole should remain", jitterMAD, restored)
	}
}

func TestEvenSchedulePeakIsIdeal(t *testing.T) {
	phases := make([]core.Phase, 60)
	for i := range phases {
		phases[i] = core.Phase(float64(i) * twoPi / 60)
	}
	peak, _ := Schedule(phases, 1, 60)
	if peak != 1 {
		t.Fatalf("exact lattice peak = %d, want 1", peak)
	}
	phases[0] = core.Wrap(float64(phases[0]) + 1e-8)
	peak, _ = Schedule(phases, 1, 60)
	if peak != 1 {
		t.Fatalf("dust-shifted lattice peak = %d, want 1", peak)
	}

	wide := make([]core.Phase, 100)
	for i := range wide {
		wide[i] = core.Phase(float64(i) * twoPi / 100)
	}
	peak, _ = Schedule(wide, 1, 60)
	if peak != IdealPeak(100, 1, 60) {
		t.Fatalf("100 nodes over 60 slots peak = %d, want %d", peak, IdealPeak(100, 1, 60))
	}
}

func TestPeakOverlapHalfOpen(t *testing.T) {
	if got := PeakOverlap([]float64{0, 0.5}, 0.5); got != 1 {
		t.Fatalf("touching intervals peak = %d, want 1", got)
	}
	if got := PeakOverlap([]float64{0, 0, 0}, 1); got != 3 {
		t.Fatalf("stacked intervals peak = %d, want 3", got)
	}
}
