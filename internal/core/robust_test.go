package core

import (
	"math"
	"testing"
)

func TestCircularMedianIgnoresOneOutlier(t *testing.T) {
	phases := []Phase{0, 0, 0, 0, 0, 2}
	if got := CircularMedian(phases); got != 0 {
		t.Fatalf("median = %v, want 0", got)
	}
}

func TestCircularMedianStaysWithTheClusterAcrossTheCut(t *testing.T) {
	// Three samples sit next to 0, one of them just below 2π. A linear
	// median of the raw numbers would land far from that cluster.
	phases := []Phase{Wrap(-0.2), 0, 0.2, 0.3}
	got := CircularMedian(phases)
	if float64(got) > 0.5 {
		t.Fatalf("median = %v, want a phase in the cluster near 0", got)
	}
}

func TestCircularMedianOfOneIsThatPhase(t *testing.T) {
	if got := CircularMedian([]Phase{1.25}); got != 1.25 {
		t.Fatalf("median = %v, want 1.25", got)
	}
	if got := CircularMedian(nil); got != 0 {
		t.Fatalf("empty median = %v, want 0", got)
	}
}

func TestMedianKuramotoDoesNotFollowOneFalsePhase(t *testing.T) {
	const dt = 0.01
	own := Phase(0)
	neighbors := make([]Observation, 10)
	for i := range neighbors {
		neighbors[i] = Observation{Phase: own}
	}
	neighbors[9].Phase = 1

	mean := NewKuramoto(1, 2).Step(own, neighbors, dt)
	median := NewMedianKuramoto(1, 2).Step(own, neighbors, dt)
	free := Wrap(float64(own) + (twoPi/1)*dt)

	if median != free {
		t.Fatalf("median step = %v, want pure rotation %v", median, free)
	}
	if float64(mean) <= float64(free) {
		t.Fatalf("mean step = %v, want a pull toward the false phase past %v", mean, free)
	}
}

func TestMedianKuramotoMatchesMeanForOneNeighbor(t *testing.T) {
	own := Phase(0.4)
	neighbors := []Observation{{ID: "a", Phase: 1.7}}
	mean := Kuramoto{Omega: 0.5, K: 2}.Step(own, neighbors, 0.1)
	median := MedianKuramoto{Omega: 0.5, K: 2}.Step(own, neighbors, 0.1)
	if math.Abs(float64(mean)-float64(median)) > 1e-12 {
		t.Fatalf("one neighbor: mean %v, median %v", mean, median)
	}
}

func TestMedianKuramotoSyncsHonestMembers(t *testing.T) {
	k := NewMedianKuramoto(1, 2)
	phases := spreadPhases(8)
	dt := 0.01
	for tick := 0; tick < 4000; tick++ {
		phases = stepPopulation(k, phases, dt)
	}
	r, _ := Order(phases)
	if r < 0.95 {
		t.Fatalf("after 4000 ticks r = %v, want > 0.95", r)
	}
}
