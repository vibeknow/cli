package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vibeknow/cli/internal/errs"
)

// deadSessionProvider is a refreshable provider whose refresh always reports
// the session as permanently gone — replaced on another device, account
// disabled, and so on.
type deadSessionProvider struct{ forceCalls atomic.Int32 }

func (p *deadSessionProvider) Token(context.Context) (string, error) { return "stale-token", nil }
func (p *deadSessionProvider) TokenType() string                     { return "oauth" }
func (p *deadSessionProvider) ForceRefresh(context.Context) (string, error) {
	p.forceCalls.Add(1)
	return "", &errs.Object{SchemaVersion: "1", Code: "session_expired", Message: "session expired; run `vibeknow auth login`"}
}

// TestRefreshRetry_SessionDeadKeepsItsCode covers the second door into the
// bug that missing credentials came through: an error that a lower layer had
// already classified being flattened into a retryable network error.
//
// The path is the 401 retry. The stored token is refused, the middleware
// force-refreshes, and the refresh reports the session permanently dead. That
// error is returned straight out of RoundTrip, where http.Client wraps it in
// *url.Error — so unless the transport layer looks through the wrapper, the
// user is told the network failed and to try again, when what actually
// happened is that their session was killed and only logging in will fix it.
func TestRefreshRetry_SessionDeadKeepsItsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := &deadSessionProvider{}
	c := New(srv.URL).WithEnvelope().WithTransport(StandardChain(p, nil))

	err := c.Do(context.Background(), "GET", "/v1/anything", nil, &struct{}{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if p.forceCalls.Load() != 1 {
		t.Fatalf("ForceRefresh called %d times, want 1", p.forceCalls.Load())
	}

	var o *errs.Object
	if !errors.As(err, &o) {
		t.Fatalf("error carries no structured code: %v", err)
	}
	if o.Code != "session_expired" {
		t.Fatalf("code = %q, want session_expired (got %v)", o.Code, err)
	}
	if o.Retryable {
		t.Errorf("a dead session must not be reported as retryable: %v", err)
	}
	if ExitCodeForCode(o.Code) != 3 {
		t.Errorf("exit code for %q = %d, want 3", o.Code, ExitCodeForCode(o.Code))
	}
}

// countingReader records how many bytes have been read from it so far.
type countingReader struct {
	r    io.Reader
	read atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read.Add(int64(n))
	return n, err
}

// probeTransport records the state of the world at the moment the request
// reaches the bottom of the middleware chain.
type probeTransport struct {
	readAtDispatch int64
	counter        *countingReader
	status         int
}

func (t *probeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.readAtDispatch = t.counter.read.Load()
	// Drain whatever the caller sent, as a real transport would.
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
	}
	return &http.Response{
		StatusCode: t.status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

// TestRefreshRetry_DoesNotBufferUnreplayableBodies pins that a streaming
// request body is passed through rather than materialised.
//
// The retry needs a replayable body, and the middleware got one by reading
// the entire request into memory first. That is free for a small JSON body,
// but document upload streams the file through an io.Pipe — so the buffering
// pulled the whole file into RAM and undid the streaming it was built on. A
// large deck would be held twice over for a retry that almost never fires,
// because the token is refreshed ahead of the request when it is near expiry.
//
// Requests that net/http can replay on its own (GetBody is set for in-memory
// bodies) still retry; ones it cannot are sent as-is.
func TestRefreshRetry_DoesNotBufferUnreplayableBodies(t *testing.T) {
	const size = 1 << 20 // 1 MiB stands in for "a document"
	counter := &countingReader{r: strings.NewReader(strings.Repeat("x", size))}

	req, err := http.NewRequest("POST", "http://example.invalid/upload", counter)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// A streaming body: net/http cannot regenerate it, so GetBody is nil —
	// exactly the shape DoUpload produces with its io.Pipe.
	req.Body = io.NopCloser(counter)
	req.GetBody = nil

	probe := &probeTransport{counter: counter, status: http.StatusOK}
	rt := RefreshRetryMiddleware{Provider: &deadSessionProvider{}}.Wrap(probe)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()

	if probe.readAtDispatch != 0 {
		t.Errorf("middleware consumed %d bytes of a %d-byte streaming body before dispatch; "+
			"an unreplayable body must be passed through, not buffered",
			probe.readAtDispatch, size)
	}
}
