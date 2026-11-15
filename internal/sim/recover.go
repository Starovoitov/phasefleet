package sim

import "fmt"

// Point is one sample of the live-gap deviation after a kill.
type Point struct {
	Strategy string
	Tick     int
	MAD      float64
	Alive    int
}

// Recovery kills a fraction of a converged DESYNC fleet and counts the
// coupling ticks until live gaps are back within relTol of 2π/N.
// Jitter keeps the initial phases, so the same kill leaves the holes in place.
func Recovery(n int, seed int64, alpha, kill float64, max int, relTol float64) (ticks int, converged bool, jitterMAD float64, series []Point, err error) {
	desync, err := New(Config{
		N:        n,
		D:        1,
		T:        1,
		Strategy: StrategyDesync,
		Alpha:    alpha,
		Seed:     seed,
		KillAt:   -1,
	})
	if err != nil {
		return 0, false, 0, nil, err
	}
	if _, ok := desync.Relax(max, relTol); !ok {
		return 0, false, 0, nil, fmt.Errorf("desync did not converge before the kill")
	}
	desync.KillFraction(kill)
	series = append(series, Point{Strategy: StrategyDesync, Tick: 0, MAD: desync.GapMAD(), Alive: desync.nAlive})
	for s := 1; s <= max; s++ {
		desync.Tick()
		series = append(series, Point{Strategy: StrategyDesync, Tick: s, MAD: desync.GapMAD(), Alive: desync.nAlive})
		if desync.Uniform(relTol) {
			ticks = s
			converged = true
			break
		}
	}
	if !converged {
		ticks = max
	}

	jitter, err := New(Config{
		N:        n,
		D:        1,
		T:        1,
		Strategy: StrategyJitter,
		Seed:     seed,
		KillAt:   -1,
	})
	if err != nil {
		return 0, false, 0, nil, err
	}
	jitter.KillFraction(kill)
	jitterMAD = jitter.GapMAD()
	series = append(series, Point{Strategy: StrategyJitter, Tick: 0, MAD: jitterMAD, Alive: jitter.nAlive})
	limit := ticks
	if limit < 1 {
		limit = 1
	}
	for s := 1; s <= limit; s++ {
		jitter.Tick()
	}
	series = append(series, Point{Strategy: StrategyJitter, Tick: limit, MAD: jitter.GapMAD(), Alive: jitter.nAlive})
	return ticks, converged, jitterMAD, series, nil
}
