package httpclient

import (
	"context"
	"net/http"
)

type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

type AuthMiddleware struct{ Provider TokenProvider }

func (m AuthMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if m.Provider != nil {
			tok, err := m.Provider.Token(r.Context())
			if err == nil && tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		return next.RoundTrip(r)
	})
}
