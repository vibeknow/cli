package credential

import (
	"encoding/json"
	"time"
)

// Status constants for three-level token status.
const (
	StatusValid        = "valid"
	StatusNeedsRefresh = "needs_refresh"
	StatusExpired      = "expired"
)

// refreshWindow is the threshold before expiry at which a token is considered
// to need refresh rather than being fully valid.
const refreshWindow = 5 * time.Minute

// safetyMargin is subtracted from OAuth expiry times to account for clock skew
// and network latency.
const safetyMargin = 30 * time.Second

// StoredToken represents the credential JSON stored in keychain (or file).
type StoredToken struct {
	Version          string    `json:"version"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
}

// ParseStored parses a raw keychain value into a StoredToken.
// If the value is valid JSON containing an "access_token" field it is parsed
// as a full OAuth token. Otherwise the raw string is treated as a PAT.
func ParseStored(raw string) StoredToken {
	var tok StoredToken
	if err := json.Unmarshal([]byte(raw), &tok); err == nil && tok.AccessToken != "" {
		return tok
	}
	// Fall back to treating the raw value as a plain PAT.
	return NewPATToken(raw)
}

// Status returns the three-level status of the token:
//   - StatusValid        — token is usable right now
//   - StatusNeedsRefresh — access token is nearly/already expired but refresh token is still valid
//   - StatusExpired      — both tokens are expired (or oauth token with no expiry info)
func (t StoredToken) Status() string {
	// PAT tokens have no expiry — always valid.
	if t.TokenType == "pat" {
		return StatusValid
	}

	now := time.Now().UTC()

	// Legacy / incomplete tokens (no expiry fields set) are treated as valid.
	if t.ExpiresAt.IsZero() && t.RefreshExpiresAt.IsZero() {
		return StatusValid
	}

	// If refresh expiry has passed, the whole credential is dead.
	if !t.RefreshExpiresAt.IsZero() && !now.Before(t.RefreshExpiresAt) {
		return StatusExpired
	}

	// If we are within the refresh window (or past expiry), signal needs_refresh.
	if !t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt.Add(-refreshWindow)) {
		return StatusNeedsRefresh
	}

	return StatusValid
}

// Marshal serializes the token to JSON.
func (t StoredToken) Marshal() []byte {
	data, _ := json.Marshal(t)
	return data
}

// NewOAuthToken creates a StoredToken from an OAuth API response.
// expiresIn and refreshExpiresIn are in seconds. A 30-second safety margin is
// subtracted from each expiry time.
func NewOAuthToken(accessToken, refreshToken string, expiresIn, refreshExpiresIn int) StoredToken {
	now := time.Now().UTC()
	return StoredToken{
		Version:          "1",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "oauth",
		ExpiresAt:        now.Add(time.Duration(expiresIn)*time.Second - safetyMargin),
		RefreshExpiresAt: now.Add(time.Duration(refreshExpiresIn)*time.Second - safetyMargin),
	}
}

// NewPATToken creates a StoredToken for a Personal Access Token.
// PATs have no refresh mechanism and no expiry.
func NewPATToken(token string) StoredToken {
	return StoredToken{
		Version:     "1",
		AccessToken: token,
		TokenType:   "pat",
	}
}
