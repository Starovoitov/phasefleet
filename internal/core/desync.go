package core

import "math"

// DESYNC moves a node toward the midpoint of the arc between the neighbor
// behind it and the neighbor ahead of it on the phase circle.
//
// Gain is the fraction of the remaining distance to that midpoint taken on
// this tick. 0 is free rotation. 1 lands on the midpoint in one call.
// Omega is the natural frequency in radians per unit time, so the phase
// keeps turning and can cross 0. dt scales only that rotation; the fraction
// itself is per call.
//
// No neighbor at a different phase means pure rotation. Equal gaps are the
// equilibrium. A negative Kuramoto coupling is a different stepper: it can
// hold two antipodal clumps, and those clumps do not spread the load.
type DESYNC struct {
	Omega float64
	Gain  float64
}

// NewDESYNC returns a stepper with natural frequency ω₀ = 2π/period.
// period uses the same time unit as dt in Step. gain is the per-tick fraction.
func NewDESYNC(period, gain float64) DESYNC {
	return DESYNC{Omega: twoPi / period, Gain: gain}
}

// Step implements Stepper.
func (d DESYNC) Step(own Phase, neighbors []Observation, dt float64) Phase {
	rot := d.Omega * dt
	if d.Gain == 0 {
		return Wrap(float64(own) + rot)
	}
	pred, succ, ok := flanks(own, neighbors)
	if !ok {
		return Wrap(float64(own) + rot)
	}
	shift := d.Gain * Delta(own, arcMidpoint(pred, succ))
	return Wrap(float64(own) + shift + rot)
}

// flanks returns the nearest phase behind own and the nearest phase ahead.
// A neighbor sitting on own is not a flank. ok is false when none remain.
func flanks(own Phase, neighbors []Observation) (pred, succ Phase, ok bool) {
	var predFwd, succFwd float64
	for _, nb := range neighbors {
		fwd := float64(Wrap(float64(nb.Phase) - float64(own)))
		if fwd == 0 {
			continue
		}
		if !ok || fwd < succFwd {
			succFwd = fwd
			succ = nb.Phase
		}
		if !ok || fwd > predFwd {
			predFwd = fwd
			pred = nb.Phase
		}
		ok = true
	}
	return pred, succ, ok
}

// arcMidpoint is the center of the forward arc from pred to succ, the arc
// that contains own. One distinct neighbor makes pred and succ the same
// point, so the arc is the full circle and the center is the antipode.
func arcMidpoint(pred, succ Phase) Phase {
	arc := float64(Wrap(float64(succ) - float64(pred)))
	if arc == 0 {
		return Wrap(float64(pred) + math.Pi)
	}
	return Wrap(float64(pred) + arc/2)
}

// Compile-time check that DESYNC is a Stepper.
var _ Stepper = DESYNC{}
