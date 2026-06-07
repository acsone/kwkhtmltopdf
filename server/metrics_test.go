package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func resetMetricsRegistry() {
	defaultRegistry = &registry{}
}

func TestMetricsHandler_ExportsCounterAndHistogram(t *testing.T) {
	resetMetricsRegistry()

	reqTotal := NewCounterVec(CounterOpts{Name: "test_requests_total", Help: "Total requests."}, []string{"path"})
	duration := NewHistogramVec(HistogramOpts{Name: "test_duration_seconds", Help: "Duration.", Buckets: []float64{0.1, 0.5}}, []string{"path"})

	reqTotal.WithLabelValues("/pdf").Inc()
	reqTotal.WithLabelValues("/pdf").Add(2)

	duration.WithLabelValues("/pdf").Observe(0.05)
	duration.WithLabelValues("/pdf").Observe(0.2)
	duration.WithLabelValues("/pdf").Observe(1.0)

	rr := httptest.NewRecorder()
	MetricsHandler(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("unexpected Content-Type: %q", ct)
	}

	// Counter
	if !strings.Contains(body, "# TYPE test_requests_total counter\n") {
		t.Fatalf("missing counter type line, got:\n%s", body)
	}
	if !strings.Contains(body, "test_requests_total{path=\"/pdf\"} 3\n") {
		t.Fatalf("missing/incorrect counter sample, got:\n%s", body)
	}

	// Histogram
	if !strings.Contains(body, "# TYPE test_duration_seconds histogram\n") {
		t.Fatalf("missing histogram type line, got:\n%s", body)
	}
	if !strings.Contains(body, "test_duration_seconds_bucket{path=\"/pdf\",le=\"0.1\"} 1\n") {
		t.Fatalf("missing/incorrect histogram bucket le=0.1, got:\n%s", body)
	}
	if !strings.Contains(body, "test_duration_seconds_bucket{path=\"/pdf\",le=\"0.5\"} 2\n") {
		t.Fatalf("missing/incorrect histogram bucket le=0.5, got:\n%s", body)
	}
	if !strings.Contains(body, "test_duration_seconds_bucket{path=\"/pdf\",le=\"+Inf\"} 3\n") {
		t.Fatalf("missing/incorrect histogram bucket le=+Inf, got:\n%s", body)
	}
	if !strings.Contains(body, "test_duration_seconds_count{path=\"/pdf\"} 3\n") {
		t.Fatalf("missing/incorrect histogram count, got:\n%s", body)
	}
	if !strings.Contains(body, "test_duration_seconds_sum{path=\"/pdf\"} 1.25\n") {
		t.Fatalf("missing/incorrect histogram sum, got:\n%s", body)
	}
}

func TestCounter_NegativeAddIgnored(t *testing.T) {
	resetMetricsRegistry()

	cv := NewCounterVec(CounterOpts{Name: "test_counter_total", Help: "help"}, []string{"k"})
	c := cv.WithLabelValues("v")
	c.Add(-10)

	rr := httptest.NewRecorder()
	MetricsHandler(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, "test_counter_total{k=\"v\"} 0\n") {
		t.Fatalf("expected counter to stay at 0, got:\n%s", body)
	}
}

func TestEscaping_LabelValuesAndHelp(t *testing.T) {
	resetMetricsRegistry()

	// Include characters that must be escaped in Prometheus text format.
	help := "line1\\line2\nline3"
	cv := NewCounterVec(CounterOpts{Name: "test_escape_total", Help: help}, []string{"k"})
	cv.WithLabelValues("a\"b\\c\nd").Inc()

	rr := httptest.NewRecorder()
	MetricsHandler(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, "# HELP test_escape_total line1\\\\line2\\nline3\n") {
		t.Fatalf("help escaping mismatch, got:\n%s", body)
	}
	if !strings.Contains(body, "test_escape_total{k=\"a\\\"b\\\\c\\nd\"} 1\n") {
		t.Fatalf("label escaping mismatch, got:\n%s", body)
	}
}

func TestEncodeLabelValues_AvoidsAmbiguity(t *testing.T) {
	// This is a regression test for common ambiguous concatenation bugs.
	k1 := encodeLabelValues([]string{"a", "bc"})
	k2 := encodeLabelValues([]string{"ab", "c"})
	if k1 == k2 {
		t.Fatalf("expected different encoded keys, got %q", k1)
	}
}

func TestMetrics_ConcurrentUpdates(t *testing.T) {
	resetMetricsRegistry()

	workers := 16
	iters := 2000

	cv := NewCounterVec(CounterOpts{Name: "test_concurrent_counter_total", Help: "help"}, []string{"worker"})
	hv := NewHistogramVec(HistogramOpts{Name: "test_concurrent_hist_seconds", Help: "help", Buckets: []float64{0.1, 0.5}}, []string{"worker"})

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		workerLabel := strconv.Itoa(w)
		go func() {
			defer wg.Done()
			c := cv.WithLabelValues(workerLabel)
			h := hv.WithLabelValues(workerLabel)
			for i := 0; i < iters; i++ {
				c.Inc()
				h.Observe(0.2)
			}
		}()
	}
	wg.Wait()

	// Export once to ensure formatting still works after concurrent writes.
	rr := httptest.NewRecorder()
	MetricsHandler(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	// Spot-check a few workers for deterministic values.
	for _, w := range []int{0, workers - 1} {
		workerLabel := strconv.Itoa(w)
		if !strings.Contains(body, "test_concurrent_counter_total{worker=\""+workerLabel+"\"} "+strconv.Itoa(iters)+"\n") {
			t.Fatalf("missing/incorrect counter for worker=%s, got:\n%s", workerLabel, body)
		}
		if !strings.Contains(body, "test_concurrent_hist_seconds_count{worker=\""+workerLabel+"\"} "+strconv.Itoa(iters)+"\n") {
			t.Fatalf("missing/incorrect histogram count for worker=%s, got:\n%s", workerLabel, body)
		}
	}
}
