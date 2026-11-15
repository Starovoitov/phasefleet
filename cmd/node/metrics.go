package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Starovoitov/phasefleet/internal/core"
)

// phaseOffset is θ − ω₀·t folded onto [0, 2π). A member that only rotates
// at its natural frequency holds a constant value. The step does not read it.
func phaseOffset(phase core.Phase, omega float64, elapsed time.Duration) float64 {
	return float64(core.Wrap(float64(phase) - omega*elapsed.Seconds()))
}

type phaseMetrics struct {
	offset  prometheus.Gauge
	handler http.Handler
}

func newPhaseMetrics() *phaseMetrics {
	offset := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "phasefleet_node_phase_offset_radians",
		Help: "Phase minus ω₀·t since the member started, wrapped onto [0, 2π).",
	})
	reg := prometheus.NewRegistry()
	reg.MustRegister(offset)
	return &phaseMetrics{
		offset:  offset,
		handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	}
}

func (m *phaseMetrics) set(v float64) {
	m.offset.Set(v)
}

func serveMetrics(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errc
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
