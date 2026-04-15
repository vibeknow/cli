package httpclient

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
)

type TraceIDMiddleware struct{}

func (TraceIDMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Trace-Id") == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			tid := "cli-" + hex.EncodeToString(buf)
			r.Header.Set("X-Trace-Id", tid)
			if os.Getenv("VIBEKNOW_TRACE") == "1" {
				r.Header.Set("X-Trace-Id-Display", tid)
			}
		}
		return next.RoundTrip(r)
	})
}
