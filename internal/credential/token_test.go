package credential

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseStored_JSON(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour)
	refreshExpiresAt := now.Add(24 * time.Hour)

	raw, _ := json.Marshal(map[string]interface{}{
		"version":            "1",
		"access_token":       "access-abc",
		"refresh_token":      "refresh-xyz",
		"token_type":         "oauth",
		"expires_at":         expiresAt.Format(time.RFC3339),
		"refresh_expires_at": refreshExpiresAt.Format(time.RFC3339),
	})

	tok := ParseStored(string(raw))

	if tok.Version != "1" {
		t.Errorf("expected version '1', got %q", tok.Version)
	}
	if tok.AccessToken != "access-abc" {
		t.Errorf("expected access_token 'access-abc', got %q", tok.AccessToken)
	}
	if tok.RefreshToken != "refresh-xyz" {
		t.Errorf("expected refresh_token 'refresh-xyz', got %q", tok.RefreshToken)
	}
	if tok.TokenType != "oauth" {
		t.Errorf("expected token_type 'oauth', got %q", tok.TokenType)
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
	if tok.RefreshExpiresAt.IsZero() {
		t.Error("expected RefreshExpiresAt to be set")
	}
}

func TestParseStored_PlainString(t *testing.T) {
	plain := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.plain.token"

	tok := ParseStored(plain)

	if tok.AccessToken != plain {
		t.Errorf("expected access_token to be the plain string, got %q", tok.AccessToken)
	}
	if tok.TokenType != "pat" {
		t.Errorf("expected token_type 'pat', got %q", tok.TokenType)
	}
	if !tok.ExpiresAt.IsZero() {
		t.Errorf("expected ExpiresAt to be zero for PAT, got %v", tok.ExpiresAt)
	}
	if !tok.RefreshExpiresAt.IsZero() {
		t.Errorf("expected RefreshExpiresAt to be zero for PAT, got %v", tok.RefreshExpiresAt)
	}
	if tok.RefreshToken != "" {
		t.Errorf("expected no RefreshToken for PAT, got %q", tok.RefreshToken)
	}
}

func TestStoredToken_Status_Valid(t *testing.T) {
	now := time.Now().UTC()
	tok := StoredToken{
		AccessToken:      "access",
		RefreshToken:     "refresh",
		TokenType:        "oauth",
		ExpiresAt:        now.Add(1 * time.Hour),
		RefreshExpiresAt: now.Add(24 * time.Hour),
	}

	status := tok.Status()
	if status != StatusValid {
		t.Errorf("expected %q, got %q", StatusValid, status)
	}
}

func TestStoredToken_Status_NeedsRefresh(t *testing.T) {
	now := time.Now().UTC()
	// expires in 3 minutes — within the 5-minute window
	tok := StoredToken{
		AccessToken:      "access",
		RefreshToken:     "refresh",
		TokenType:        "oauth",
		ExpiresAt:        now.Add(3 * time.Minute),
		RefreshExpiresAt: now.Add(24 * time.Hour),
	}

	status := tok.Status()
	if status != StatusNeedsRefresh {
		t.Errorf("expected %q, got %q", StatusNeedsRefresh, status)
	}
}

func TestStoredToken_Status_Expired(t *testing.T) {
	now := time.Now().UTC()
	tok := StoredToken{
		AccessToken:      "access",
		RefreshToken:     "refresh",
		TokenType:        "oauth",
		ExpiresAt:        now.Add(-2 * time.Hour),
		RefreshExpiresAt: now.Add(-30 * time.Minute),
	}

	status := tok.Status()
	if status != StatusExpired {
		t.Errorf("expected %q, got %q", StatusExpired, status)
	}
}

func TestStoredToken_Status_PAT(t *testing.T) {
	tok := NewPATToken("my-personal-access-token")

	status := tok.Status()
	if status != StatusValid {
		t.Errorf("PAT should always be valid, got %q", status)
	}
}

func TestStoredToken_Marshal(t *testing.T) {
	original := NewOAuthToken("access-tok", "refresh-tok", 3600, 86400)

	data := original.Marshal()
	if len(data) == 0 {
		t.Fatal("Marshal returned empty bytes")
	}

	// Round-trip: parse back
	parsed := ParseStored(string(data))

	if parsed.AccessToken != original.AccessToken {
		t.Errorf("access_token mismatch: got %q, want %q", parsed.AccessToken, original.AccessToken)
	}
	if parsed.RefreshToken != original.RefreshToken {
		t.Errorf("refresh_token mismatch: got %q, want %q", parsed.RefreshToken, original.RefreshToken)
	}
	if parsed.TokenType != original.TokenType {
		t.Errorf("token_type mismatch: got %q, want %q", parsed.TokenType, original.TokenType)
	}
	// Allow up to 1 second difference due to serialization rounding
	if diff := parsed.ExpiresAt.Sub(original.ExpiresAt); diff < -time.Second || diff > time.Second {
		t.Errorf("ExpiresAt mismatch: got %v, want %v (diff %v)", parsed.ExpiresAt, original.ExpiresAt, diff)
	}
	if diff := parsed.RefreshExpiresAt.Sub(original.RefreshExpiresAt); diff < -time.Second || diff > time.Second {
		t.Errorf("RefreshExpiresAt mismatch: got %v, want %v (diff %v)", parsed.RefreshExpiresAt, original.RefreshExpiresAt, diff)
	}
}

// TestNewOAuthToken_MissingExpiryIsUnknownNotExpired guards the sharpest
// failure this type can produce.
//
// The expiry fields are computed as now+expires_in. When a response omits an
// expiry — a field renamed, an endpoint that never sent it, a non-rotating
// refresh setup — that arithmetic yields now-30s, i.e. a timestamp already in
// the past. Status() then reads Expired, and the very next command purges the
// credential: one missing field in a backend response silently logs the user
// out immediately after a successful login.
//
// Unknown has to stay unknown. Status() already treats a zero expiry as
// "no information", which is the honest reading.
func TestNewOAuthToken_MissingExpiryIsUnknownNotExpired(t *testing.T) {
	cases := []struct {
		name                      string
		expiresIn, refreshExpires int
	}{
		{"no refresh expiry", 7200, 0},
		{"no access expiry", 0, 2592000},
		{"neither", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := NewOAuthToken("at", "rt", tc.expiresIn, tc.refreshExpires)
			if got := tok.Status(); got == StatusExpired {
				t.Errorf("Status() = %q for a freshly issued token; a missing expiry must not read as expired "+
					"(expires_at=%v refresh_expires_at=%v)", got, tok.ExpiresAt, tok.RefreshExpiresAt)
			}
		})
	}
}
