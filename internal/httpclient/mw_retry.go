package httpclient

import (
	"net/http"
	"time"
)

// RetryMiddleware retries 5xx responses with exponential backoff.
// MaxAttempts counts the initial attempt; 3 means "try + 2 retries".
type RetryMiddleware struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func (m RetryMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		attempts := m.MaxAttempts
		if attempts < 1 {
			attempts = 1
		}
		delay := m.BaseDelay
		if delay <= 0 {
			delay = 100 * time.Millisecond
		}
		var resp *http.Response
		var err error
		for i := 0; i < attempts; i++ {
			resp, err = next.RoundTrip(r)
			if err == nil && !is5xx(resp.StatusCode) {
				return resp, nil
			}
			if resp != nil {
				resp.Body.Close()
			}
			if i == attempts-1 {
				break
			}
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
		return resp, err
	})
}
