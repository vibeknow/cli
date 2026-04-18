package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The doctor expects every backend to be running go-atlas ≥ v0.3.6, which
// exposes /healthz under the service base group and responds with
// {"status":"healthy"} on 200 or {"status":"unhealthy"} on 503.

func TestProbeHealthHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "healthy",
			"pillars": map[string]any{"db": map[string]string{"status": "healthy", "latency": "1ms"}},
		})
	}))
	defer srv.Close()

	ok, detail := probeHealth(srv.URL)
	if !ok {
		t.Fatalf("want ok, got fail: %s", detail)
	}
}

func TestProbeHealthUnhealthyReturns503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "unhealthy"})
	}))
	defer srv.Close()

	ok, detail := probeHealth(srv.URL)
	if ok {
		t.Fatal("want fail, got ok")
	}
	if !strings.Contains(detail, "http=503") {
		t.Errorf("detail should record 503, got %q", detail)
	}
}

func TestProbeHealth404IsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	ok, detail := probeHealth(srv.URL)
	if ok {
		t.Fatal("want fail, got ok")
	}
	if !strings.Contains(detail, "http=404") {
		t.Errorf("detail should record 404, got %q", detail)
	}
}

func TestProbeHealthTransportErrorIsFail(t *testing.T) {
	// Port 1 never listens — connection refused.
	ok, detail := probeHealth("http://127.0.0.1:1")
	if ok {
		t.Fatal("want fail on unreachable host")
	}
	if detail == "" {
		t.Error("detail should describe the transport error")
	}
}
