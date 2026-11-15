package sink

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Interval bounds in seconds. The last finite bound is one hour;
// longer gaps still count in the histogram total.
var intervalBounds = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5,
	1, 2.5, 5, 10, 30, 60, 300, 3600,
}

var (
	concurrencyDesc = prometheus.NewDesc(
		"phasefleet_sink_concurrency",
		"Arrivals whose task is still in progress at scrape time.",
		nil, nil,
	)
	peakDesc = prometheus.NewDesc(
		"phasefleet_sink_peak",
		"Highest concurrency since the sink started.",
		nil, nil,
	)
	intervalDesc = prometheus.NewDesc(
		"phasefleet_sink_interval_seconds",
		"Gap between consecutive arrivals.",
		nil, nil,
	)
)

// Describe implements prometheus.Collector.
func (s *Sink) Describe(ch chan<- *prometheus.Desc) {
	ch <- concurrencyDesc
	ch <- peakDesc
	ch <- intervalDesc
}

// Collect implements prometheus.Collector. Concurrency uses the sink clock
// at scrape time, so a late scrape does not invent arrivals.
func (s *Sink) Collect(ch chan<- prometheus.Metric) {
	now := time.Now()
	s.mu.Lock()
	conc := concurrency(s.at, s.d, now)
	peak := s.peak
	gaps := intervals(s.at)
	s.mu.Unlock()

	ch <- prometheus.MustNewConstMetric(concurrencyDesc, prometheus.GaugeValue, float64(conc))
	ch <- prometheus.MustNewConstMetric(peakDesc, prometheus.GaugeValue, float64(peak))
	count, sum, buckets := gapHistogram(gaps)
	ch <- prometheus.MustNewConstHistogram(intervalDesc, count, sum, buckets)
}

func gapHistogram(gaps []float64) (count uint64, sum float64, buckets map[float64]uint64) {
	buckets = make(map[float64]uint64, len(intervalBounds))
	count = uint64(len(gaps))
	for _, g := range gaps {
		if g > 0 {
			sum += g
		}
	}
	for _, bound := range intervalBounds {
		var n uint64
		for _, g := range gaps {
			if g <= bound {
				n++
			}
		}
		buckets[bound] = n
	}
	return count, sum, buckets
}
