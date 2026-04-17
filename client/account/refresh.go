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
