package httpclient

import (
	"context"
	"net/http"
)

type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// StaticToken is a TokenProvider that always returns the same bearer token
// and never refreshes. Use it for env-var PATs or tokens already resolved
// from the keychain when refresh isn't needed.
type StaticToken string

func (s StaticToken) Token(_ context.Context) (string, error) { return string(s), nil }

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
