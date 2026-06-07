package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", MetricsHandler)
	mux.HandleFunc("/", handler)
	mux.HandleFunc("/pdf", handler)
	mux.HandleFunc("/image", handler)
	return mux
}

func TestMetricsEndpoint_AllowsGet(t *testing.T) {
	mux := testServerMux()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /metrics, got %d", rr.Code)
	}
}

func TestMetricsEndpointTrailingSlash_AllowsGet(t *testing.T) {
	mux := testServerMux()

	req := httptest.NewRequest(http.MethodGet, "/metrics/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /metrics/, got %d", rr.Code)
	}
}
