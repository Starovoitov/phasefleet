package sink

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPeakAndIntervals(t *testing.T) {
	d := 100 * time.Millisecond
	s := New(d)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Out of order on purpose: the record sorts by time.
	s.Arrive(base.Add(200 * time.Millisecond))
	s.Arrive(base)
	s.Arrive(base.Add(50 * time.Millisecond))
	s.Arrive(base.Add(60 * time.Millisecond))

	if s.Peak() != 3 {
		t.Fatalf("peak = %d, want 3", s.Peak())
	}
	if got := s.Concurrency(base.Add(60 * time.Millisecond)); got != 3 {
		t.Fatalf("concurrency at 60ms = %d, want 3", got)
	}
	// The arrival at 0 covers [0, 100ms). At 100ms it has ended.
	if got := s.Concurrency(base.Add(d)); got != 2 {
		t.Fatalf("concurrency at 100ms = %d, want 2", got)
	}
	if got := s.Concurrency(base.Add(200 * time.Millisecond)); got != 1 {
		t.Fatalf("concurrency at 200ms = %d, want 1", got)
	}

	want := []float64{0.05, 0.01, 0.14}
	got := s.Intervals()
	if len(got) != len(want) {
		t.Fatalf("intervals = %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("intervals = %v, want %v", got, want)
		}
	}

	srv := httptest.NewServer(Handler(s))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if got := metricValue(t, string(body), "phasefleet_sink_peak"); got != 3 {
		t.Fatalf("phasefleet_sink_peak = %v, want 3", got)
	}
	if got := metricValue(t, string(body), "phasefleet_sink_interval_seconds_count"); got != 3 {
		t.Fatalf("interval count = %v, want 3", got)
	}
}

func TestHalfOpenEndpointDoesNotOverlap(t *testing.T) {
	d := 100 * time.Millisecond
	s := New(d)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Arrive(base)
	s.Arrive(base.Add(d))
	if s.Peak() != 1 {
		t.Fatalf("peak = %d, want 1", s.Peak())
	}
	if got := s.Concurrency(base.Add(d)); got != 1 {
		t.Fatalf("concurrency at the endpoint = %d, want 1", got)
	}
}

func TestPostShowsPeakInMetrics(t *testing.T) {
	s := New(time.Second)
	srv := httptest.NewServer(Handler(s))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/fire", "application/json", strings.NewReader(`{"id":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %s", resp.Status)
	}
	if s.Peak() != 1 {
		t.Fatalf("peak = %d, want 1", s.Peak())
	}

	resp, err = http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if got := metricValue(t, string(body), "phasefleet_sink_peak"); got != 1 {
		t.Fatalf("phasefleet_sink_peak = %v, want 1", got)
	}

	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Peak      int       `json:"peak"`
		Intervals []float64 `json:"intervals"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if snap.Peak != 1 {
		t.Fatalf("snapshot peak = %d", snap.Peak)
	}
	if snap.Intervals == nil {
		t.Fatal("intervals should be an empty list, not null")
	}
}

func TestPostRejectsMissingID(t *testing.T) {
	s := New(time.Second)
	srv := httptest.NewServer(Handler(s))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/fire", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s", resp.Status)
	}
	if s.Peak() != 0 {
		t.Fatalf("peak = %d, want 0", s.Peak())
	}
}

func metricValue(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name+" ") && !strings.HasPrefix(line, name+"{") {
			continue
		}
		fields := strings.Fields(line)
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Fatalf("metric %s not found", name)
	return 0
}
