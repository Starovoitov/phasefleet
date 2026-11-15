package core

// Desync moves one phase toward the middle of the arc between the
// neighbors immediately ahead and behind, then adds the natural rotation.
//
// Ahead is the smallest positive step forward on the circle. Behind is
// the smallest positive step backward. One other phase fills both roles,
// so a pair is driven to opposite sides. Alpha is the fraction of that
// centering move taken on this tick. Zero leaves free rotation. An empty
// neighborhood, or one where every observation shares own phase, also
// leaves free rotation.
type Desync struct {
	Omega float64
	Alpha float64
}

// NewDesync returns a stepper with natural frequency ω₀ = 2π/period.
// period uses the same time unit as dt in Step. alpha is the per-tick
// fraction of the move toward the midpoint, normally in (0, 1].
func NewDesync(period, alpha float64) Desync {
	return Desync{Omega: twoPi / period, Alpha: alpha}
}

// Step implements Stepper.
func (d Desync) Step(own Phase, neighbors []Observation, dt float64) Phase {
	advanced := float64(own) + d.Omega*dt
	if d.Alpha == 0 {
		return Wrap(advanced)
	}
	ahead, behind, ok := neighborGaps(own, neighbors)
	if !ok {
		return Wrap(advanced)
	}
	shift := d.Alpha * (ahead - behind) / 2
	return Wrap(advanced + shift)
}

// neighborGaps reports the forward gap to the nearest phase ahead and the
// backward gap to the nearest phase behind. ok is false when no observation
// sits off own phase.
func neighborGaps(own Phase, neighbors []Observation) (ahead, behind float64, ok bool) {
	bestAhead := twoPi
	bestBehind := twoPi
	for _, nb := range neighbors {
		fw := float64(Wrap(float64(nb.Phase) - float64(own)))
		if fw == 0 {
			continue
		}
		bk := twoPi - fw
		if fw < bestAhead {
			bestAhead = fw
		}
		if bk < bestBehind {
			bestBehind = bk
		}
		ok = true
	}
	if !ok {
		return 0, 0, false
	}
	return bestAhead, bestBehind, true
}

// Compile-time check that Desync is a Stepper.
var _ Stepper = Desync{}
