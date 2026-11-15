package core

import (
	"math"
	"testing"
)

func TestDESYNCMovesFractionTowardMidpoint(t *testing.T) {
	const (
		gain  = 0.25
		omega = 0.1
		dt    = 0.2
	)
	own := Phase(0)
	// Successor just ahead, predecessor opposite. The center of the arc
	// that contains own is 3π/2 + 0.1, a backward step of π/2 - 0.1.
	succ := Phase(0.2)
	pred := Phase(math.Pi)
	mid := math.Pi + (math.Pi+0.2)/2
	delta := mid - twoPi
	want := float64(Wrap(float64(own) + gain*delta + omega*dt))

	got := DESYNC{Omega: omega, Gain: gain}.Step(own, []Observation{
		{ID: "far", Phase: 1}, // between the flanks, must be ignored
		{ID: "succ", Phase: succ},
		{ID: "pred", Phase: pred},
		{ID: "succ-again", Phase: succ},
	}, dt)
	if math.Abs(float64(got)-want) > 1e-12 {
		t.Fatalf("Step = %v, want %v", got, want)
	}
}

func TestDESYNCSingleNeighborTargetsAntipode(t *testing.T) {
	const nb = 0.4
	got := DESYNC{Gain: 1}.Step(0, []Observation{{Phase: nb}}, 1)
	want := float64(Wrap(nb + math.Pi))
	if math.Abs(float64(got)-want) > 1e-12 {
		t.Fatalf("Step = %v, want antipode %v", got, want)
	}
}

func TestDESYNCFreeRotation(t *testing.T) {
	d := NewDESYNC(1, 0.8)
	start := Phase(0.3)
	neighbors := []Observation{{Phase: 2.5}, {Phase: 4}}
	free := DESYNC{Omega: d.Omega, Gain: 0}

	got := start
	noGain := start
	const steps = 1000
	dt := 1.0 / steps
	for i := 0; i < steps; i++ {
		got = d.Step(got, nil, dt)
		noGain = free.Step(noGain, neighbors, dt)
	}
	if math.Abs(float64(got)-float64(start)) > 1e-9 {
		t.Fatalf("after one period with no neighbors phase = %v, want %v", got, start)
	}
	if math.Abs(float64(noGain)-float64(start)) > 1e-9 {
		t.Fatalf("gain 0 drifted to %v, want %v", noGain, start)
	}
	if math.Abs(float64(got)-float64(noGain)) > 1e-12 {
		t.Fatalf("empty neighborhood and gain 0 diverged: %v vs %v", got, noGain)
	}
}

func TestDESYNCKeepsEqualGaps(t *testing.T) {
	const n = 6
	phases := make([]Phase, n)
	for i := range phases {
		phases[i] = Phase(float64(i) * twoPi / float64(n))
	}
	d := NewDESYNC(1, 1)
	dt := 0.01
	for tick := 0; tick < 400; tick++ {
		phases = stepPopulation(d, phases, dt)
	}
	if mad := gapMAD(phases); mad > 1e-9 {
		t.Fatalf("gap MAD = %v, equal spacing drifted; phases %v", mad, phases)
	}
}

func TestDESYNCEqualizesGaps(t *testing.T) {
	d := NewDESYNC(1, 0.5)
	phases := []Phase{0.01, 0.04, 0.07, 0.11, 0.16, 0.2, 0.28, 0.35}
	const ticks = 2000
	const dt = 0.01
	for tick := 0; tick < ticks; tick++ {
		phases = stepPopulation(d, phases, dt)
	}
	const tol = 1e-3
	if mad := gapMAD(phases); mad > tol {
		t.Fatalf("gap MAD = %v, want <= %v; phases %v", mad, tol, phases)
	}
}
