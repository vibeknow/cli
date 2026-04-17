package httpclient

import (
	"bytes"
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

		// Buffer the request body so it can be replayed on retry.
		var bodyBytes []byte
		if r.Body != nil && r.Body != http.NoBody {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
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
		if len(bodyBytes) > 0 {
			r2.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		return next.RoundTrip(r2)
	})
}
