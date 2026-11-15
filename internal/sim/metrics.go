package sim

import (
	"math"
	"sort"

	"github.com/Starovoitov/phasefleet/internal/core"
)

// IdealPeak is ⌈N·d/T⌉, the overlap of a perfectly even schedule.
// Intervals are half-open, so an exact integer N·d/T stays on that integer.
func IdealPeak(n int, d, period float64) int {
	if n <= 0 || period <= 0 || d <= 0 {
		return 0
	}
	return int(math.Ceil(float64(n)*d/period - 1e-9))
}

// SteadyPeak is the overlap of one period once phases are frozen and only
// the natural rotation remains. A dust tolerance keeps a gap that is short
// by a rounding error from counting as a real overlap.
func (f *Fleet) SteadyPeak() (peak int, median float64) {
	return Schedule(f.livePhases(), f.cfg.D, f.cfg.T)
}

// Schedule places one firing per phase in a single period and measures it
// on the circle. Phase 0 fires at time T, with the others.
func Schedule(phases []core.Phase, d, period float64) (peak int, median float64) {
	arrivals := make([]float64, len(phases))
	for i, p := range phases {
		theta := float64(p)
		frac := 1.0
		if theta != 0 {
			frac = (twoPi - theta) / twoPi
		}
		arrivals[i] = frac * period
	}
	return CircularPeak(arrivals, d, period), circularMedian(arrivals, period)
}

// CircularPeak counts the busiest instant on a repeating period.
func CircularPeak(arrivals []float64, d, period float64) int {
	n := len(arrivals)
	if n == 0 || d <= 0 || period <= 0 {
		return 0
	}
	if d >= period {
		return n
	}
	// 0.1% of the mean spacing. Enough to ignore a settled fleet's rounding,
	// and far smaller than the gap that would really add another overlap.
	dEff := d - 1e-3*period/float64(n)
	if dEff <= 0 {
		return 1
	}
	type event struct {
		t     float64
		delta int
	}
	ev := make([]event, 0, n*2)
	for _, raw := range arrivals {
		t := math.Mod(raw, period)
		if t < 0 {
			t += period
		}
		end := t + dEff
		if end < period {
			ev = append(ev, event{t, 1}, event{end, -1})
			continue
		}
		ev = append(ev, event{t, 1}, event{0, 1}, event{end - period, -1})
	}
	sort.Slice(ev, func(i, j int) bool {
		if ev[i].t != ev[j].t {
			return ev[i].t < ev[j].t
		}
		return ev[i].delta < ev[j].delta
	})
	cur, max := 0, 0
	for _, e := range ev {
		cur += e.delta
		if cur > max {
			max = cur
		}
	}
	return max
}

func circularMedian(arrivals []float64, period float64) float64 {
	if len(arrivals) < 2 {
		return 0
	}
	s := append([]float64(nil), arrivals...)
	sort.Float64s(s)
	gaps := make([]float64, len(s))
	for i := 0; i < len(s)-1; i++ {
		gaps[i] = s[i+1] - s[i]
	}
	gaps[len(s)-1] = s[0] + period - s[len(s)-1]
	sort.Float64s(gaps)
	mid := len(gaps) / 2
	if len(gaps)%2 == 1 {
		return gaps[mid]
	}
	return (gaps[mid-1] + gaps[mid]) / 2
}

// PeakOverlap is the maximum number of half-open intervals [t, t+d) that
// cover one instant. An endpoint ends one task before the next starts.
func PeakOverlap(arrivals []float64, d float64) int {
	if d <= 0 || len(arrivals) == 0 {
		return 0
	}
	type event struct {
		t     float64
		delta int
	}
	ev := make([]event, 0, len(arrivals)*2)
	for _, t := range arrivals {
		ev = append(ev, event{t, 1}, event{t + d, -1})
	}
	sort.Slice(ev, func(i, j int) bool {
		if ev[i].t != ev[j].t {
			return ev[i].t < ev[j].t
		}
		return ev[i].delta < ev[j].delta
	})
	cur, max := 0, 0
	for _, e := range ev {
		cur += e.delta
		if cur > max {
			max = cur
		}
	}
	return max
}

// MedianInterval is the median gap between arrivals sorted in time.
func MedianInterval(arrivals []float64) float64 {
	if len(arrivals) < 2 {
		return 0
	}
	s := append([]float64(nil), arrivals...)
	sort.Float64s(s)
	gaps := make([]float64, len(s)-1)
	for i := range gaps {
		gaps[i] = s[i+1] - s[i]
	}
	sort.Float64s(gaps)
	mid := len(gaps) / 2
	if len(gaps)%2 == 1 {
		return gaps[mid]
	}
	return (gaps[mid-1] + gaps[mid]) / 2
}

func gapMAD(phases []core.Phase) float64 {
	n := len(phases)
	if n < 2 {
		return 0
	}
	s := make([]float64, n)
	for i, p := range phases {
		s[i] = float64(p)
	}
	sort.Float64s(s)
	ideal := twoPi / float64(n)
	var mad float64
	for i := 0; i < n; i++ {
		var g float64
		if i == n-1 {
			g = s[0] + twoPi - s[i]
		} else {
			g = s[i+1] - s[i]
		}
		mad += math.Abs(g - ideal)
	}
	return mad / float64(n)
}
