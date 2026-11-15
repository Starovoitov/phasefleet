package core

// Observation is a neighbor phase as known to one node.
// The simulator applies link delay before building this slice, so a step
// never sees a clock or an RTT matrix.
type Observation struct {
	ID    string
	Phase Phase
}

// Stepper advances one node by one tick.
// own is the current phase. neighbors are the observations available on this tick.
// dt is the step length in the same time unit as the natural period.
//
// K > 0 (sync) and repulsion (desync) are different steppers.
// A negative Kuramoto coupling drives the order parameter toward 0,
// which two antipodal clumps satisfy without spreading load.
// Uniform spacing is a separate stepper (DESYNC), not negative K.
type Stepper interface {
	Step(own Phase, neighbors []Observation, dt float64) Phase
}
