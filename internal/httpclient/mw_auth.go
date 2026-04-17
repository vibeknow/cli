package httpclient

import (
	"context"
	"net/http"
)

type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// RefreshableTokenProvider extends TokenProvider with refresh capability.
type RefreshableTokenProvider interface {
	TokenProvider
	TokenType() string                               // "oauth" or "pat"
	ForceRefresh(ctx context.Context) (string, error) // force refresh, return new access_token
}

type AuthMiddleware struct{ Provider TokenProvider }

func (m AuthMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if m.Provider != nil {
			tok, err := m.Provider.Token(r.Context())
			if err == nil && tok != "" {
				r.Header.Set("X-Authorization-Token", tok)
			}
		}
		return next.RoundTrip(r)
	})
}
