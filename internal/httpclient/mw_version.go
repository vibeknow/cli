package httpclient

import "net/http"

// VersionMiddleware verifies the X-Vibeknow-Api-Version response header matches
// Expected (CLI's compile-time version). Missing header is tolerated (warning-only)
// for services that haven't adopted the header yet.
type VersionMiddleware struct {
	Expected string
}

func (m VersionMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := next.RoundTrip(r)
		if err != nil || resp == nil {
			return resp, err
		}
		got := resp.Header.Get("X-Vibeknow-Api-Version")
		if got == "" {
			return resp, nil
		}
		if got != m.Expected {
			resp.Body.Close()
			return nil, &errObject{
				Code:      "version_mismatch",
				Message:   "server API version " + got + " incompatible with CLI version " + m.Expected + "; please update the CLI",
				Retryable: false,
			}
		}
		return resp, nil
	})
}
