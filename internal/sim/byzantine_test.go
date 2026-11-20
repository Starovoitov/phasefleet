package sim

import (
	"math"
	"testing"

	"github.com/Starovoitov/phasefleet/internal/core"
)

func TestLockedPhaseStaysPut(t *testing.T) {
	for _, strategy := range []string{StrategyKuramoto, StrategyDesync} {
		res, err := Run(Config{
			N: 5, D: 1, T: 1, Dt: 0.01, Ticks: 30,
			Strategy: strategy, K: 2, Alpha: 0.5, Seed: 1, KillAt: -1,
			Lock: true, LockIndex: 4, LockedPhase: 1.25,
		})
		if err != nil {
			t.Fatal(err)
		}
		got := res.Final[4]
		if !got.Alive {
			t.Fatalf("%s locked member died", strategy)
		}
		if math.Abs(float64(got.Phase)-1.25) > 1e-12 {
			t.Fatalf("%s locked phase = %v, want 1.25", strategy, got.Phase)
		}
	}
}

func TestLockIndexAndMedianAggregate(t *testing.T) {
	_, err := New(Config{
		N: 3, T: 1, Strategy: StrategyKuramoto, KillAt: -1,
		Lock: true, LockIndex: 3,
	})
	if err == nil {
		t.Fatal("expected lock index to be rejected")
	}
	_, err = New(Config{
		N: 4, T: 1, Strategy: StrategyDesync, Alpha: 0.5, KillAt: -1,
		Aggregate: "median",
	})
	if err == nil {
		t.Fatal("expected median to be refused for desync")
	}
	_, err = New(Config{
		N: 4, T: 1, Strategy: StrategyKuramoto, K: 2, KillAt: -1,
		Aggregate: "nope",
	})
	if err == nil {
		t.Fatal("expected unknown aggregate to be rejected")
	}
}

func TestByzantineMeanFollowsAndMedianDoesNot(t *testing.T) {
	const seed = 1
	mean, median, err := Byzantine(seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(mean) != byzantineTicks+1 || len(median) != len(mean) {
		t.Fatalf("samples mean=%d median=%d", len(mean), len(median))
	}
	// The second half is one natural period.
	from := len(mean) / 2
	meanAdv := unwrappedAdvance(mean[from:])
	medianAdv := unwrappedAdvance(median[from:])
	if math.Abs(meanAdv) > 0.35 {
		t.Fatalf("mean advance over the last period = %v, want about 0 (entrained to the fixed phase)", meanAdv)
	}
	if math.Abs(medianAdv-twoPi) > 0.2 {
		t.Fatalf("median advance over the last period = %v, want about 2π", medianAdv)
	}
	if mean[len(mean)-1].Dist > 0.5 {
		t.Fatalf("mean final distance = %v, want < 0.5", mean[len(mean)-1].Dist)
	}
	if mean[len(mean)-1].R < 0.99 || median[len(median)-1].R < 0.99 {
		t.Fatalf("honest r mean=%v median=%v, want > 0.99", mean[len(mean)-1].R, median[len(median)-1].R)
	}
}

func unwrappedAdvance(samples []ByzantineSample) float64 {
	if len(samples) < 2 {
		return 0
	}
	var adv float64
	prev := samples[0].Psi
	for _, s := range samples[1:] {
		adv += core.Delta(core.Phase(prev), core.Phase(s.Psi))
		prev = s.Psi
	}
	return adv
}
