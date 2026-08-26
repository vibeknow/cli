package httpclient

import (
	"io"
	"net/http"
)

// RefreshRetryMiddleware retries a request once on 401 if the provider
// implements RefreshableTokenProvider and the token type is "oauth".
type RefreshRetryMiddleware struct {
	Provider TokenProvider
}

func (m RefreshRetryMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		rp, ok := m.Provider.(RefreshableTokenProvider)
		if !ok {
			// Provider doesn't support refresh — pass through.
			return next.RoundTrip(r)
		}

		resp, err := next.RoundTrip(r)
		if err != nil {
			return resp, err
		}

		// Only retry on 401 for oauth tokens.
		if resp.StatusCode != http.StatusUnauthorized {
			return resp, nil
		}
		if rp.TokenType() != "oauth" {
			return resp, nil
		}

		// A retry needs a second copy of the request body. net/http supplies
		// GetBody for bodies it can regenerate — every in-memory body this
		// CLI sends, which is all of them except one.
		//
		// The exception is document upload, which streams the file through an
		// io.Pipe. Buffering the request up front to make it replayable held
		// the entire file in memory and cancelled out the streaming it was
		// written for, on the chance of a 401 that is already made unlikely
		// by refreshing ahead of the request. An unreplayable body is now
		// sent once and its 401 handed back, which the caller surfaces as the
		// auth error it is.
		hasBody := r.Body != nil && r.Body != http.NoBody
		if hasBody && r.GetBody == nil {
			return resp, nil
		}

		// Drain and close the original response body before retrying.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		// Force-refresh the token.
		newToken, err := rp.ForceRefresh(r.Context())
		if err != nil {
			return nil, err
		}

		// Clone the request and set the new token.
		r2 := r.Clone(r.Context())
		r2.Header.Set("X-Authorization-Token", newToken)
		if hasBody {
			body, err := r.GetBody()
			if err != nil {
				return nil, err
			}
			r2.Body = body
		}

		return next.RoundTrip(r2)
	})
}
