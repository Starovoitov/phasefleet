package sim

import (
	"math"
	"math/rand/v2"

	"github.com/Starovoitov/phasefleet/internal/core"
)

const (
	byzantineN     = 11
	byzantineLock  = 10
	byzantineK     = 200
	byzantineT     = 1.0
	byzantineDt    = 0.001
	byzantineTicks = 2000
)

// ByzantineSample is the honest clump on one tick.
// Dist is the arc from the honest mean phase to the locked phase.
// Psi is that mean phase. R is the order parameter of the honest members.
type ByzantineSample struct {
	Tick int
	Dist float64
	R    float64
	Psi  float64
}

// Byzantine runs one locked member twice on a fixed seed: once under the
// Kuramoto mean, once under the circular median. Ten honest members start
// as a tight clump at 0.4. The locked member publishes π and does not step.
// The second half of each series is one natural period.
//
// The mean drags the clump onto the locked phase and, because that phase
// does not rotate, stops the clump's own rotation. The median leaves the
// clump on ω₀.
func Byzantine(seed int64) (mean, median []ByzantineSample, err error) {
	mean, err = runByzantine(seed, "mean")
	if err != nil {
		return nil, nil, err
	}
	median, err = runByzantine(seed, "median")
	if err != nil {
		return nil, nil, err
	}
	return mean, median, nil
}

func runByzantine(seed int64, aggregate string) ([]ByzantineSample, error) {
	f, err := New(Config{
		N:           byzantineN,
		D:           1,
		T:           byzantineT,
		Dt:          byzantineDt,
		Strategy:    StrategyKuramoto,
		K:           byzantineK,
		Seed:        seed,
		KillAt:      -1,
		Aggregate:   aggregate,
		Lock:        true,
		LockIndex:   byzantineLock,
		LockedPhase: math.Pi,
	})
	if err != nil {
		return nil, err
	}
	f.plantHonestClump(seed)
	out := make([]ByzantineSample, 0, byzantineTicks+1)
	out = append(out, f.byzantineSample(0))
	for tick := 1; tick <= byzantineTicks; tick++ {
		f.Tick()
		out = append(out, f.byzantineSample(tick))
	}
	return out, nil
}

func (f *Fleet) plantHonestClump(seed int64) {
	rng := rand.New(rand.NewPCG(uint64(seed), 1))
	for i := range f.phase {
		if f.locked(i) {
			continue
		}
		f.phase[i] = core.Wrap(0.4 + (rng.Float64()-0.5)*0.02)
	}
}

func (f *Fleet) byzantineSample(tick int) ByzantineSample {
	honest := make([]core.Phase, 0, len(f.phase)-1)
	for i, p := range f.phase {
		if f.locked(i) {
			continue
		}
		honest = append(honest, p)
	}
	r, psi := core.Order(honest)
	dist := core.Delta(core.Phase(psi), f.phase[f.cfg.LockIndex])
	if dist < 0 {
		dist = -dist
	}
	return ByzantineSample{Tick: tick, Dist: dist, R: r, Psi: psi}
}
