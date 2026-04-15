package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type tokenProviderStub struct{ tok string }

func (t tokenProviderStub) Token(ctx context.Context) (string, error) { return t.tok, nil }

func TestWhoami(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/profile" {
			http.Error(w, "wrong path", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok_xyz" {
			http.Error(w, "no auth", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":      "u_123",
			"nickname": "alice",
			"email":    "alice@example.com",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, tokenProviderStub{"tok_xyz"})
	u, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.UID != "u_123" || u.Nickname != "alice" {
		t.Errorf("unexpected user: %+v", u)
	}
}
