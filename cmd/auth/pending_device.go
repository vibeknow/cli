package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
)

// pendingDeviceFile is where an in-flight device authorization is parked.
const pendingDeviceFile = "pending-device-auth.json"

// pendingDevice is a device authorization that has been handed to the user
// but not yet exchanged for a token.
//
// It exists because the process that starts a device login is not
// guaranteed to be alive when the user finishes authorizing in the browser.
// A host that spawns `auth login --headless` may kill it on its own
// schedule — WorkBuddy allows the auth subprocess five minutes, while the
// device code itself is good for fifteen — and when that happens the only
// copy of the device code dies with the process. The user authorizes
// successfully, and the CLI never finds out: the browser says "done" while
// the connector sits at "not connected", and the only way forward is to
// start over and authorize a second time.
//
// Parking the code on disk turns that dead end into a resumable state. Any
// later `auth status` can finish the exchange, which is exactly the flow
// the WorkBuddy connector spec describes for device-code CLIs (§12, 方案 B:
// "status 命令执行时从服务端拉取 token") and exactly what the reconnect
// path does anyway — it runs `status` before it runs `auth`.
type pendingDevice struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	Interval        int       `json:"interval"`
	ExpiresAt       time.Time `json:"expires_at"`
	Profile         string    `json:"profile"`
	AccountURL      string    `json:"account_url"`
}

// expired reports whether the device code is past its life. An expired
// record is worse than useless: exchanging it can only fail, and keeping it
// makes `status` report a pending authorization that will never land.
func (p pendingDevice) expired() bool {
	return p.ExpiresAt.IsZero() || !time.Now().Before(p.ExpiresAt)
}

func pendingDevicePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, pendingDeviceFile), nil
}

// savePendingDevice parks an in-flight device authorization.
//
// The file holds a live credential — whoever has the device code can claim
// the token the moment the user authorizes — so it is written 0600 into the
// config dir rather than anywhere world-readable.
func savePendingDevice(p pendingDevice) error {
	path, err := pendingDevicePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// loadPendingDevice returns the parked authorization, if any. A missing,
// unreadable or corrupt file is reported as "nothing pending" rather than
// as an error: this is a best-effort resume path, and failing `auth status`
// because of it would break the very flow it exists to repair.
func loadPendingDevice() (pendingDevice, bool) {
	path, err := pendingDevicePath()
	if err != nil {
		return pendingDevice{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// Unreadable is treated the same as absent, deliberately.
			return pendingDevice{}, false
		}
		return pendingDevice{}, false
	}
	var p pendingDevice
	if err := json.Unmarshal(b, &p); err != nil || p.DeviceCode == "" {
		return pendingDevice{}, false
	}
	return p, true
}

// clearPendingDevice removes the parked authorization. Called once the code
// has been spent or has died, so a stale code cannot keep a "pending"
// signal alive forever.
func clearPendingDevice() {
	path, err := pendingDevicePath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

// pendingExchangeTimeout bounds the resume attempt made from `auth status`.
//
// The connector spec gives `status` ten seconds and polls it every three,
// so the resume attempt has to be decisively cheaper than the command it is
// attached to. Two seconds is enough for one round trip to the token
// endpoint and short enough that a hung network costs a poll, not the
// connection: a status that overruns its budget reads as "not connected"
// and would flap the very state this is trying to settle.
const pendingExchangeTimeout = 2 * time.Second

// resumePendingDeviceLogin tries once to turn a parked device code into a
// stored token, and reports whether an authorization is still outstanding.
//
// Returns (completed, stillPending):
//   - (true, false)  the user authorized; the token is now in the keychain
//   - (false, true)  the user has not authorized yet; try again next poll
//   - (false, false) nothing was parked, or the code is dead and was cleared
//
// Every failure mode short of "authorized" leaves the caller's own view of
// authentication untouched. This runs inside a status check that must keep
// working when the network does not, so it reports rather than raises.
func resumePendingDeviceLogin(ctx context.Context) (completed, stillPending bool) {
	rec, ok := loadPendingDevice()
	if !ok {
		return false, false
	}
	if rec.expired() {
		clearPendingDevice()
		return false, false
	}

	ctx, cancel := context.WithTimeout(ctx, pendingExchangeTimeout)
	defer cancel()

	client := account.NewUnauthenticated(rec.AccountURL)
	tokenResp, err := client.DeviceToken(ctx, rec.DeviceCode)
	if err != nil {
		var pollErr *account.PollError
		if errors.As(err, &pollErr) {
			switch pollErr.Status {
			case account.PollExpired, account.PollDenied:
				// Terminal: the code will never be exchangeable.
				clearPendingDevice()
				return false, false
			}
			// Pending or slow_down: the user simply has not finished yet.
			return false, true
		}
		// A transport failure says nothing about the authorization, so the
		// record stays parked for the next attempt.
		return false, true
	}

	profile := pendingProfile(rec)
	st := credential.NewOAuthToken(
		tokenResp.AccessToken,
		tokenResp.RefreshToken,
		tokenResp.ExpiresIn,
		tokenResp.RefreshExpiresIn,
	)
	if err := storeToken(profile, st); err != nil {
		// The code is spent — a second exchange would fail — but the token
		// did not land. Clear it so `status` stops promising a resume that
		// cannot happen, and let the user log in again.
		clearPendingDevice()
		return false, false
	}
	clearPendingDevice()
	return true, false
}

// pendingProfile resolves the profile the parked login belongs to. The
// record names it; if that profile has since been removed, fall back to the
// same default `auth login` would have used, reconstructed with the account
// URL the code was actually issued against.
func pendingProfile(rec pendingDevice) config.Profile {
	if f, err := config.LoadProfiles(); err == nil {
		for _, p := range f.Profiles {
			if p.Name == rec.Profile {
				return p
			}
		}
	}
	p, _, err := resolveAccountURL()
	if err == nil {
		return p
	}
	return config.Profile{
		Name:          "default",
		CredentialRef: "vibeknow.default",
		Trust:         "user",
		IsProduction:  true,
	}
}
