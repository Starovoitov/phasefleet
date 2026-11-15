package core

import "math"

// Kuramoto is one Euler step of the Kuramoto oscillator.
//
//	dθ/dt = ω₀ + (K/n) Σ sin(θⱼ − θᵢ)
//
// Omega is ω₀ in radians per unit time. K is the coupling. n is the number
// of observations on this tick, so a neighbor that has not arrived yet is
// left out of the sum. An empty neighborhood is pure rotation.
//
// K > 0 pulls phases together. K = 0 rotates freely: aligned, jitter, and
// hash schedules use that after their initial phase is chosen. K < 0 drives
// the order parameter toward 0. Two antipodal clumps satisfy that, so a
// negative K is not a uniform spacing of the fleet.
type Kuramoto struct {
	Omega float64
	K     float64
}

// NewKuramoto returns a stepper with natural frequency ω₀ = 2π/period.
// period uses the same time unit as dt in Step. K is the coupling.
func NewKuramoto(period, k float64) Kuramoto {
	return Kuramoto{Omega: twoPi / period, K: k}
}

// Step implements Stepper.
func (k Kuramoto) Step(own Phase, neighbors []Observation, dt float64) Phase {
	deriv := k.Omega
	if k.K != 0 && len(neighbors) > 0 {
		var sum float64
		for _, nb := range neighbors {
			sum += math.Sin(Delta(own, nb.Phase))
		}
		deriv += k.K / float64(len(neighbors)) * sum
	}
	return Wrap(float64(own) + deriv*dt)
}

// Compile-time check that Kuramoto is a Stepper.
var _ Stepper = Kuramoto{}
