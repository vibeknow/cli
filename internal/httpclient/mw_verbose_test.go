package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerboseLogsSummaryWithRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	chain := Chain(http.DefaultTransport,
		AuthMiddleware{Provider: staticTokenProvider("secret-token-shh")},
		VerboseMiddleware{Out: &buf},
	)
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "POST", "/x", map[string]string{"k": "v"}, nil)

	log := buf.String()
	if !strings.Contains(log, "POST") || !strings.Contains(log, "/x") {
		t.Errorf("log missing method/path: %q", log)
	}
	if strings.Contains(log, "secret-token-shh") {
		t.Errorf("log leaked token: %q", log)
	}
}

func TestVerboseDisabledProducesNoOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	// Out is nil (zero value) — middleware is a no-op; no panic, no output.
	chain := Chain(http.DefaultTransport, VerboseMiddleware{})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	// Nothing to assert on a nil Out; just verify no panic and request completes.
}
