package core

import (
	"math"
	"sort"
)

// CircularMedian is a phase that minimizes the sum of arc lengths to the
// samples. An empty slice returns 0. When several phases share that sum,
// the smallest one is returned, so the result does not depend on input order.
//
// One sample far from the rest cannot move the median while the others
// outnumber it. A member that has gone silent is simply absent from the
// slice; the median does not invent a phase for it.
func CircularMedian(phases []Phase) Phase {
	n := len(phases)
	if n == 0 {
		return 0
	}
	sorted := append([]Phase(nil), phases...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	best := sorted[0]
	bestCost := math.Inf(1)
	consider := func(m Phase) {
		cost := 0.0
		for _, p := range sorted {
			d := Delta(m, p)
			if d < 0 {
				d = -d
			}
			cost += d
		}
		if cost < bestCost-1e-12 || (cost <= bestCost+1e-12 && m < best) {
			bestCost = cost
			best = m
		}
	}
	for _, p := range sorted {
		consider(p)
	}
	// An even count can put the minimum anywhere on the arc between two
	// samples. The midpoint of that arc is the candidate checked here.
	if n%2 == 0 {
		for i := 0; i < n; i++ {
			a := float64(sorted[i])
			b := float64(sorted[(i+1)%n])
			if i == n-1 {
				b += twoPi
			}
			consider(Wrap((a + b) / 2))
		}
	}
	return best
}

// MedianKuramoto is the Kuramoto step with the neighbor mean replaced by
// the circular median. The pull is K·sin(median − own), not the average of
// sin(θⱼ − θᵢ). Identical natural frequencies still lock the honest members
// to each other. A single false phase does not, while it remains the minority.
type MedianKuramoto struct {
	Omega float64
	K     float64
}

// NewMedianKuramoto returns a median stepper with ω₀ = 2π/period.
// period uses the same time unit as dt in Step. K is the coupling.
func NewMedianKuramoto(period, k float64) MedianKuramoto {
	return MedianKuramoto{Omega: twoPi / period, K: k}
}

// Step implements Stepper.
func (k MedianKuramoto) Step(own Phase, neighbors []Observation, dt float64) Phase {
	deriv := k.Omega
	if k.K != 0 && len(neighbors) > 0 {
		phases := make([]Phase, len(neighbors))
		for i, nb := range neighbors {
			phases[i] = nb.Phase
		}
		deriv += k.K * math.Sin(Delta(own, CircularMedian(phases)))
	}
	return Wrap(float64(own) + deriv*dt)
}

// Compile-time check that MedianKuramoto is a Stepper.
var _ Stepper = MedianKuramoto{}
