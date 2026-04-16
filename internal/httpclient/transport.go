package httpclient

import (
	"io"
	"net/http"
	"time"
)

// Middleware wraps a RoundTripper with additional behavior.
type Middleware interface {
	Wrap(next http.RoundTripper) http.RoundTripper
}

// Chain applies middlewares in order: Chain(base, A, B, C) yields
// A(B(C(base))) — A sees the request first, C sends it last.
func Chain(base http.RoundTripper, mws ...Middleware) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := base
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i].Wrap(rt)
	}
	return rt
}

// roundTripperFunc adapts a function to RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// RoundTripperFunc adapts a function to http.RoundTripper. Exported for use by
// service clients that need custom middleware (e.g., vectoria's X-API-Key).
func RoundTripperFunc(fn func(*http.Request) (*http.Response, error)) http.RoundTripper {
	return roundTripperFunc(fn)
}

// StandardChain returns the canonical middleware stack used by all service
// clients: Auth → TraceID → Verbose → Version → Retry. Pass nil verboseOut
// to disable verbose logging.
func StandardChain(tp TokenProvider, verboseOut io.Writer) http.RoundTripper {
	return Chain(http.DefaultTransport,
		AuthMiddleware{Provider: tp},
		TraceIDMiddleware{},
		VerboseMiddleware{Out: verboseOut},
		VersionMiddleware{Expected: ClientAPIVersion},
		RetryMiddleware{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond},
	)
}
