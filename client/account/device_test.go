package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeviceCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/auth/device/code" {
			http.Error(w, "wrong path/method", 404)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		if body["client_id"] != "vibeknow-cli" || body["scope"] != "full" {
			http.Error(w, "wrong body", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"device_code":      "dev_abc",
				"user_code":        "USR-123",
				"verification_uri": "https://vibeknow.com/activate",
				"expires_in":       900,
				"interval":         5,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.DeviceCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.DeviceCode != "dev_abc" {
		t.Errorf("DeviceCode: got %q, want %q", resp.DeviceCode, "dev_abc")
	}
	if resp.UserCode != "USR-123" {
		t.Errorf("UserCode: got %q, want %q", resp.UserCode, "USR-123")
	}
	if resp.VerificationURI != "https://vibeknow.com/activate" {
		t.Errorf("VerificationURI: got %q", resp.VerificationURI)
	}
	if resp.ExpiresIn != 900 {
		t.Errorf("ExpiresIn: got %d, want 900", resp.ExpiresIn)
	}
	if resp.Interval != 5 {
		t.Errorf("Interval: got %d, want 5", resp.Interval)
	}
}

func TestDeviceToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"access_token":       "at_xyz",
				"refresh_token":      "rt_xyz",
				"token_type":         "Bearer",
				"expires_in":         3600,
				"refresh_expires_in": 86400,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.DeviceToken(context.Background(), "dev_abc")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "at_xyz" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "at_xyz")
	}
	if resp.RefreshExpiresIn != 86400 {
		t.Errorf("RefreshExpiresIn: got %d, want 86400", resp.RefreshExpiresIn)
	}
}

func TestDeviceToken_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    40010,
			"message": "authorization_pending",
			"data":    nil,
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dev_abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *PollError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PollError, got %T: %v", err, err)
	}
	if pe.Status != PollPending {
		t.Errorf("Status: got %q, want %q", pe.Status, PollPending)
	}
}

func TestDeviceToken_SlowDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    40011,
			"message": "slow_down",
			"data":    nil,
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dev_abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *PollError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PollError, got %T: %v", err, err)
	}
	if pe.Status != PollSlowDown {
		t.Errorf("Status: got %q, want %q", pe.Status, PollSlowDown)
	}
}

func TestDeviceToken_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    40012,
			"message": "expired_token",
			"data":    nil,
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dev_abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *PollError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PollError, got %T: %v", err, err)
	}
	if pe.Status != PollExpired {
		t.Errorf("Status: got %q, want %q", pe.Status, PollExpired)
	}
}

func TestDeviceToken_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    40013,
			"message": "access_denied",
			"data":    nil,
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dev_abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *PollError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PollError, got %T: %v", err, err)
	}
	if pe.Status != PollDenied {
		t.Errorf("Status: got %q, want %q", pe.Status, PollDenied)
	}
}
