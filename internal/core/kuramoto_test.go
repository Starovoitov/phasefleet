package core

import (
	"math"
	"sort"
	"testing"
)

func TestKuramotoStepMatchesFormula(t *testing.T) {
	const (
		omega = 0.5
		k     = 2.0
		dt    = 0.1
	)
	own := Phase(0.4)
	neighbors := []Observation{
		{ID: "a", Phase: Wrap(float64(own) + math.Pi/2)},
		{ID: "b", Phase: Wrap(float64(own) + math.Pi/6)},
	}
	sum := math.Sin(math.Pi/2) + math.Sin(math.Pi/6)
	want := float64(Wrap(float64(own) + (omega+k/2*sum)*dt))

	got := Kuramoto{Omega: omega, K: k}.Step(own, neighbors, dt)
	if math.Abs(float64(got)-want) > 1e-12 {
		t.Fatalf("Step = %v, want %v", got, want)
	}
}

func TestKuramotoFreeRotation(t *testing.T) {
	k := NewKuramoto(1, 0)
	start := Phase(0.3)
	withNeighbor := []Observation{{ID: "other", Phase: 2.5}}

	got := start
	alone := start
	const steps = 1000
	dt := 1.0 / steps
	for i := 0; i < steps; i++ {
		got = k.Step(got, withNeighbor, dt)
		alone = k.Step(alone, nil, dt)
	}
	if math.Abs(float64(got)-float64(start)) > 1e-9 {
		t.Fatalf("after one period phase = %v, want %v", got, start)
	}
	if math.Abs(float64(got)-float64(alone)) > 1e-12 {
		t.Fatalf("neighbors moved a free rotor: with %v, without %v", got, alone)
	}
}

func TestKuramotoSyncs(t *testing.T) {
	// Identical natural frequencies lock for any K > 0. K = 2 is past that threshold.
	k := NewKuramoto(1, 2)
	phases := spreadPhases(8)
	dt := 0.01

	r0, _ := Order(phases)
	if r0 > 0.5 {
		t.Fatalf("initial r = %v, want a spread population below 0.5", r0)
	}

	for tick := 0; tick < 2000; tick++ {
		phases = stepPopulation(k, phases, dt)
	}
	r, _ := Order(phases)
	if r < 0.95 {
		t.Fatalf("after 2000 ticks r = %v, want > 0.95", r)
	}
}

func TestKuramotoNegativeKKeepsAntipodalClumps(t *testing.T) {
	// Exact antipodes are an equilibrium for any K. A small push shows that
	// K < 0 keeps the two clumps instead of relaxing them into equal gaps.
	k := NewKuramoto(1, -1)
	phases := []Phase{0.02, -0.02, math.Pi + 0.02, math.Pi - 0.02}
	for i := range phases {
		phases[i] = Wrap(float64(phases[i]))
	}

	dt := 0.01
	for tick := 0; tick < 2000; tick++ {
		phases = stepPopulation(k, phases, dt)
	}

	r, _ := Order(phases)
	if r > 0.05 {
		t.Fatalf("antipodal r = %v, want < 0.05", r)
	}
	mad := gapMAD(phases)
	// Equal gaps of 2π/N would put MAD near 0. Two clumps sit near π/2.
	if mad < 0.5 {
		t.Fatalf("gap MAD = %v, clumps equalized; phases %v", mad, phases)
	}
}

func spreadPhases(n int) []Phase {
	phases := make([]Phase, n)
	for i := range phases {
		phases[i] = Wrap(float64(i) * 0.7)
	}
	return phases
}

func stepPopulation(s Stepper, phases []Phase, dt float64) []Phase {
	next := make([]Phase, len(phases))
	for i, own := range phases {
		neighbors := make([]Observation, 0, len(phases)-1)
		for j, p := range phases {
			if i == j {
				continue
			}
			neighbors = append(neighbors, Observation{Phase: p})
		}
		next[i] = s.Step(own, neighbors, dt)
	}
	return next
}

func gapMAD(phases []Phase) float64 {
	if len(phases) == 0 {
		return 0
	}
	s := make([]float64, len(phases))
	for i, p := range phases {
		s[i] = float64(p)
	}
	sort.Float64s(s)
	ideal := twoPi / float64(len(s))
	var mad float64
	for i := 0; i < len(s); i++ {
		var g float64
		if i == len(s)-1 {
			g = s[0] + twoPi - s[i]
		} else {
			g = s[i+1] - s[i]
		}
		mad += math.Abs(g - ideal)
	}
	return mad / float64(len(s))
}
