package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/internal/config"
)

func TestParseHealthOKAcceptsFlat(t *testing.T) {
	if !parseHealthOK([]byte(`{"status":"ok","checks":{"db":true}}`)) {
		t.Error("flat {status:ok} should be accepted")
	}
}

func TestParseHealthOKAcceptsEnvelope(t *testing.T) {
	if !parseHealthOK([]byte(`{"code":0,"message":"response.ok","data":{"status":"ok"}}`)) {
		t.Error("envelope with data.status=ok should be accepted")
	}
}

func TestParseHealthOKAcceptsAtlasHealthy(t *testing.T) {
	// atlas returns {"status":"healthy"} not "ok"
	if !parseHealthOK([]byte(`{"status":"healthy"}`)) {
		t.Error("atlas-style {status:healthy} should be accepted")
	}
}

func TestParseHealthOKRejectsMalformed(t *testing.T) {
	cases := []string{
		`{"status":"down"}`,
		`{"code":1,"data":{"status":"ok"}}`, // envelope with non-zero code
		`not json at all`,
		`{}`,
	}
	for _, c := range cases {
		if parseHealthOK([]byte(c)) {
			t.Errorf("should reject: %s", c)
		}
	}
}

// httpServer records the paths probed so tests can assert we tried the
// expected sequence.
type httpServer struct {
	*httptest.Server
	probed []string
}

func newHTTPServer(handler http.HandlerFunc) *httpServer {
	s := &httpServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.probed = append(s.probed, r.URL.Path)
		handler(w, r)
	}))
	return s
}

func TestProbeServiceHitsFirstWorkingPath(t *testing.T) {
	// /healthz -> 200 atlas-style; /v1/health and /health should not be probed.
	srv := newHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"account": srv.URL}}
	res := probeService(prof, "account")

	if res.status != probeOK {
		t.Fatalf("status = %v, want probeOK (detail: %s)", res.status, res.detail)
	}
	if len(srv.probed) != 1 || srv.probed[0] != "/healthz" {
		t.Fatalf("expected only /healthz probed, got %v", srv.probed)
	}
	if !strings.Contains(res.detail, "path=/healthz") {
		t.Errorf("detail should report which path worked, got %q", res.detail)
	}
}

func TestProbeServiceFallsThroughTo404Chain(t *testing.T) {
	// /healthz -> 404, /v1/health -> 200 envelope.
	srv := newHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"ok"}}`))
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"vibeknow": srv.URL}}
	res := probeService(prof, "vibeknow")

	if res.status != probeOK {
		t.Fatalf("status = %v, want probeOK (detail: %s)", res.status, res.detail)
	}
	// Should have tried /healthz first, then /v1/health.
	if len(srv.probed) != 2 || srv.probed[0] != "/healthz" || srv.probed[1] != "/v1/health" {
		t.Fatalf("unexpected probe sequence: %v", srv.probed)
	}
}

func TestProbeServiceAll404WarnsNotFails(t *testing.T) {
	srv := newHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"figlens": srv.URL}}
	res := probeService(prof, "figlens")

	if res.status != probeNoHealthEndpoint {
		t.Fatalf("status = %v, want probeNoHealthEndpoint (detail: %s)", res.status, res.detail)
	}
	// Should have tried all three paths.
	if len(srv.probed) != 3 {
		t.Fatalf("expected 3 probes on all-404 host, got %v", srv.probed)
	}
	if !strings.Contains(res.detail, "health endpoint not exposed") {
		t.Errorf("detail should explain the state, got %q", res.detail)
	}
}

func TestProbeServiceHardFailureOn5xx(t *testing.T) {
	// First path returns 500 — should be recorded as a hard fail, and later
	// paths might still be tried but the 500 wins if they also don't return OK.
	srv := newHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			http.Error(w, "boom", 500)
			return
		}
		http.Error(w, "not found", 404)
	})
	defer srv.Close()

	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"account": srv.URL}}
	res := probeService(prof, "account")

	if res.status != probeFail {
		t.Fatalf("status = %v, want probeFail (detail: %s)", res.status, res.detail)
	}
	if !strings.Contains(res.detail, "http=500") {
		t.Errorf("detail should record the 500, got %q", res.detail)
	}
}

func TestProbeServiceTransportErrorIsHardFail(t *testing.T) {
	// Point at an address nothing listens on; short timeout ensures test is fast.
	// Use the invalid host scheme so the transport errors out immediately on DNS.
	prof := config.Profile{Trust: "dev", IsProduction: false, Endpoints: map[string]string{"account": "http://127.0.0.1:1"}}
	res := probeService(prof, "account")

	if res.status != probeFail {
		t.Fatalf("status = %v, want probeFail on unreachable host (detail: %s)", res.status, res.detail)
	}
}
