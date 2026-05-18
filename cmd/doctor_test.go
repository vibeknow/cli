package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The doctor expects every backend to expose /healthz (with /health as a
// fallback) and respond with {"status":"healthy"} on 200 or
// {"status":"unhealthy"} on 503. A 503 with the databases pillar still
// healthy is reported as degraded, not failed.

func TestProbeHealthHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "healthy",
			"pillars": map[string]any{"databases": map[string]string{"status": "healthy", "latency": "1ms"}},
		})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeOK {
		t.Fatalf("want probeOK, got %v: %s", status, detail)
	}
}

func TestProbeHealthFallsBackToHealthOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "healthy",
			"pillars": map[string]any{"databases": map[string]string{"status": "healthy", "latency": "1ms"}},
		})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeOK {
		t.Fatalf("want probeOK via /health fallback, got %v: %s", status, detail)
	}
}

func TestProbeHealthAcceptsAlternateStatusShape(t *testing.T) {
	// Some services don't follow the {"status":"healthy"} convention — they
	// return {"status":"ok","checks":{...}} or similar. A 200 from /healthz
	// (or /health) is enough; we shouldn't couple to a specific keyword.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"checks": map[string]bool{"database": true, "storage": true},
		})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeOK {
		t.Fatalf("want probeOK for 200 + status=ok shape, got %v: %s", status, detail)
	}
}

func TestProbeHealthDegradedWhenDBHealthyButOtherPillarDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "unhealthy",
			"pillars": map[string]any{
				"databases": map[string]string{"status": "healthy", "latency": "1ms"},
				"email":     map[string]string{"status": "unhealthy", "latency": "1ms"},
				"sms":       map[string]string{"status": "healthy", "latency": "1ms"},
			},
		})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeDegraded {
		t.Fatalf("want probeDegraded, got %v: %s", status, detail)
	}
	if !strings.Contains(detail, "email") {
		t.Errorf("detail should name the down pillar, got %q", detail)
	}
}

func TestProbeHealthFailWhenDBDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "unhealthy",
			"pillars": map[string]any{
				"databases": map[string]string{"status": "unhealthy", "latency": "1ms"},
			},
		})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeFail {
		t.Fatalf("want probeFail when DB pillar is down, got %v: %s", status, detail)
	}
}

func TestProbeHealthUnhealthyReturns503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "unhealthy"})
	}))
	defer srv.Close()

	status, detail := probeHealth(srv.URL)
	if status != probeFail {
		t.Fatalf("want probeFail (no pillars info), got %v", status)
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

	status, detail := probeHealth(srv.URL)
	if status != probeFail {
		t.Fatalf("want probeFail when both /healthz and /health 404, got %v", status)
	}
	if !strings.Contains(detail, "http=404") {
		t.Errorf("detail should record 404, got %q", detail)
	}
}

func TestProbeHealthTransportErrorIsFail(t *testing.T) {
	// Port 1 never listens — connection refused.
	status, detail := probeHealth("http://127.0.0.1:1")
	if status != probeFail {
		t.Fatalf("want probeFail on unreachable host, got %v", status)
	}
	if detail == "" {
		t.Error("detail should describe the transport error")
	}
}
