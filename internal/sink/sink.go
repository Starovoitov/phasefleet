// Package sink records task arrivals and how their durations overlap.
// It does not interpret phase. The caller supplies the arrival time;
// the HTTP handler uses the sink process clock.
package sink

import (
	"sort"
	"sync"
	"time"
)

// Sink is the load record. d is how long one arrival stays in progress.
// Intervals are half-open: an arrival at t covers [t, t+d).
type Sink struct {
	d    time.Duration
	mu   sync.Mutex
	at   []time.Time
	peak int
}

// New records arrivals that each last d.
func New(d time.Duration) *Sink {
	return &Sink{d: d}
}

// Arrive stores one arrival. The HTTP layer checks who fired; the peak
// is only the overlap of durations.
func (s *Sink) Arrive(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.at = append(s.at, at)
	if p := overlap(s.at, s.d); p > s.peak {
		s.peak = p
	}
}

// Concurrency is how many arrivals are still in progress at now.
func (s *Sink) Concurrency(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return concurrency(s.at, s.d, now)
}

// Peak is the highest concurrency since the sink was created.
func (s *Sink) Peak() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peak
}

// Intervals are the gaps, in seconds, between arrivals ordered by time.
func (s *Sink) Intervals() []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return intervals(s.at)
}

func concurrency(times []time.Time, d time.Duration, now time.Time) int {
	if d <= 0 {
		return 0
	}
	n := 0
	for _, at := range times {
		if !now.Before(at) && now.Before(at.Add(d)) {
			n++
		}
	}
	return n
}

func intervals(times []time.Time) []float64 {
	if len(times) < 2 {
		return []float64{}
	}
	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })
	gaps := make([]float64, len(sorted)-1)
	for i := range gaps {
		gaps[i] = sorted[i+1].Sub(sorted[i]).Seconds()
	}
	return gaps
}

// overlap is the busiest instant of the half-open intervals [t, t+d).
// An end sorts before a start at the same instant, so a task that ends
// as the next one begins does not count as overlap.
func overlap(times []time.Time, d time.Duration) int {
	if d <= 0 || len(times) == 0 {
		return 0
	}
	base := times[0]
	for _, t := range times[1:] {
		if t.Before(base) {
			base = t
		}
	}
	type event struct {
		t     float64
		delta int
	}
	width := float64(d)
	ev := make([]event, 0, len(times)*2)
	for _, t := range times {
		start := float64(t.Sub(base))
		ev = append(ev, event{start, 1}, event{start + width, -1})
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
