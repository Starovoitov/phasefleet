package core

import (
	"math"
	"testing"
)

func TestDesyncStepCentersArc(t *testing.T) {
	d := Desync{Omega: 0, Alpha: 1}
	own := Phase(0)
	neighbors := []Observation{
		{Phase: 1},
		{Phase: Wrap(-0.5)},
	}
	// Ahead gap is 1, behind gap is 0.5, midpoint sits 0.25 forward.
	got := d.Step(own, neighbors, 0)
	if math.Abs(float64(got)-0.25) > 1e-12 {
		t.Fatalf("Step = %v, want 0.25", got)
	}
}

func TestDesyncFreeRotation(t *testing.T) {
	start := Phase(0.3)
	neighbors := []Observation{{Phase: 2.5}}
	const steps = 1000
	dt := 1.0 / steps

	free := NewDesync(1, 0)
	alone := NewDesync(1, 0.8)
	got := start
	noNeighbors := start
	for i := 0; i < steps; i++ {
		got = free.Step(got, neighbors, dt)
		noNeighbors = alone.Step(noNeighbors, nil, dt)
	}
	if math.Abs(float64(got)-float64(start)) > 1e-9 {
		t.Fatalf("alpha 0 after one period = %v, want %v", got, start)
	}
	if math.Abs(float64(noNeighbors)-float64(start)) > 1e-9 {
		t.Fatalf("empty neighborhood after one period = %v, want %v", noNeighbors, start)
	}
}

func TestDesyncPairReachesAntipode(t *testing.T) {
	d := Desync{Omega: 0, Alpha: 0.5}
	a := Phase(0)
	b := Phase(math.Pi / 2)
	nextA := d.Step(a, []Observation{{Phase: b}}, 0)
	nextB := d.Step(b, []Observation{{Phase: a}}, 0)
	gap := math.Abs(Delta(nextA, nextB))
	if math.Abs(gap-math.Pi) > 1e-12 {
		t.Fatalf("pair gap = %v, want π (phases %v, %v)", gap, nextA, nextB)
	}
}

func TestDesyncHoldsEqualGaps(t *testing.T) {
	const n = 5
	d := NewDesync(1, 0.5)
	phases := make([]Phase, n)
	for i := range phases {
		phases[i] = Wrap(float64(i) * twoPi / n)
	}
	for tick := 0; tick < 200; tick++ {
		phases = stepPopulation(d, phases, 0.01)
	}
	if mad := gapMAD(phases); mad > 1e-9 {
		t.Fatalf("equal gaps drifted, MAD = %v, phases %v", mad, phases)
	}
}

func TestDesyncEqualizesGaps(t *testing.T) {
	d := NewDesync(1, 0.5)
	phases := []Phase{0.05, 0.4, 0.55, 1.7, 1.9, 3.4, 5.8}
	for tick := 0; tick < 400; tick++ {
		phases = stepPopulation(d, phases, 0.01)
	}
	if mad := gapMAD(phases); mad > 1e-2 {
		t.Fatalf("gap MAD = %v, want < 0.01; phases %v", mad, phases)
	}
}
