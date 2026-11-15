// Package core updates one node's phase from the phases of its neighbors.
// It does not read a clock or the network, so the simulator and a real node
// call the same step.
package core

import "math"

// Phase is an angle in radians on [0, 2π).
type Phase float64

const twoPi = 2 * math.Pi

// Wrap folds theta onto [0, 2π).
func Wrap(theta float64) Phase {
	x := math.Mod(theta, twoPi)
	if x < 0 {
		x += twoPi
	}
	if x == twoPi {
		x = 0
	}
	return Phase(x)
}

// Delta is the signed shortest step from a to b, in (-π, π].
func Delta(a, b Phase) float64 {
	d := float64(Wrap(float64(b) - float64(a)))
	if d > math.Pi {
		d -= twoPi
	}
	return d
}

// Order is the Kuramoto order parameter of a population.
// r is in [0, 1]: 1 means every phase coincides, 0 means there is no dominant phase.
// psi is the mean phase on [0, 2π). An empty population returns 0, 0.
func Order(phases []Phase) (r, psi float64) {
	if len(phases) == 0 {
		return 0, 0
	}
	var sumSin, sumCos float64
	for _, p := range phases {
		sumSin += math.Sin(float64(p))
		sumCos += math.Cos(float64(p))
	}
	n := float64(len(phases))
	sumSin /= n
	sumCos /= n
	return math.Hypot(sumCos, sumSin), float64(Wrap(math.Atan2(sumSin, sumCos)))
}
