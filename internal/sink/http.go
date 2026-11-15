package sink

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler serves POST /fire, a JSON snapshot at GET /, and Prometheus at GET /metrics.
// A firing's arrival time is the sink's clock when the request is read.
func Handler(s *Sink) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(s)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fire", func(w http.ResponseWriter, r *http.Request) {
		postFire(s, w, r)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeSnapshot(s, w)
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return mux
}

func postFire(s *Sink, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	closeErr := r.Body.Close()
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	s.Arrive(time.Now())
	slog.Info("fire", "id", got.ID, "peak", s.Peak())
	w.WriteHeader(http.StatusNoContent)
}

func writeSnapshot(s *Sink, w http.ResponseWriter) {
	now := time.Now()
	snap := struct {
		Concurrency int       `json:"concurrency"`
		Peak        int       `json:"peak"`
		Intervals   []float64 `json:"intervals"`
	}{
		Concurrency: s.Concurrency(now),
		Peak:        s.Peak(),
		Intervals:   s.Intervals(),
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		slog.Error("snapshot", "err", err)
	}
}
