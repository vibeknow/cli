package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/internal/config"
)

// TestProbeServiceAgainstAtlasHealthz simulates a service running go-atlas
// v0.3.6: /healthz at service base returns {"status":"healthy","pillars":{...}}
// with a non-trivial body. Doctor should report [ok].
func TestProbeServiceAgainstAtlasHealthz(t *testing.T) {
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

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"account": srv.URL}}
	res := probeService(prof, "account")

	if res.status != probeOK {
		t.Fatalf("status=%v detail=%s (want probeOK for atlas 0.3.6 healthz)", res.status, res.detail)
	}
}

// TestProbeServiceAgainstAtlasHealthzUnhealthy simulates atlas 0.3.6 returning
// 503 for an unhealthy service. Doctor should [fail] (not warn) — an unhealthy
// service is a real problem.
func TestProbeServiceAgainstAtlasHealthzUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "unhealthy"})
	}))
	defer srv.Close()

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"account": srv.URL}}
	res := probeService(prof, "account")

	if res.status != probeFail {
		t.Fatalf("status=%v detail=%s (want probeFail for 503 unhealthy)", res.status, res.detail)
	}
}
