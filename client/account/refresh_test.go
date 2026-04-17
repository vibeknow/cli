package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/auth/token/refresh" {
			http.Error(w, "wrong path/method", 404)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		if body["refresh_token"] != "rt_abc" {
			http.Error(w, "wrong refresh token", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"access_token":       "at_new",
				"refresh_token":      "rt_new",
				"expires_in":         3600,
				"refresh_expires_in": 86400,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.RefreshToken(context.Background(), "rt_abc")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "at_new" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "at_new")
	}
	if resp.RefreshToken != "rt_new" {
		t.Errorf("RefreshToken: got %q, want %q", resp.RefreshToken, "rt_new")
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn: got %d, want 3600", resp.ExpiresIn)
	}
	if resp.RefreshExpiresIn != 86400 {
		t.Errorf("RefreshExpiresIn: got %d, want 86400", resp.RefreshExpiresIn)
	}
}
