package account

import (
	"context"
	"net/http"
)

// RefreshTokenResponse holds the new tokens returned by the refresh endpoint.
type RefreshTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// Logout ends the session the refresh token belongs to, server-side.
// POST /v1/auth/logout
//
// Without this, disconnecting only deleted the local copy: a JWT is valid
// because it is signed, so the token stayed usable by anyone who had it for
// the rest of its life — up to ninety days on a device grant.
//
// Works on a refresh token that has already been rotated away, which matters
// because the copy in the keychain is not always the newest one.
func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	body := map[string]string{"refresh_token": refreshToken}
	return c.http.Do(ctx, http.MethodPost, "/v1/auth/logout", body, nil)
}

// RefreshToken exchanges a refresh token for a new access token.
// POST /v1/auth/token/refresh
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenResponse, error) {
	body := map[string]string{
		"refresh_token": refreshToken,
	}
	var resp RefreshTokenResponse
	if err := c.http.Do(ctx, http.MethodPost, "/v1/auth/token/refresh", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
