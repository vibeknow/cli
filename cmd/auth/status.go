package auth

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/keychain"
	"github.com/vibeknow/cli/internal/output"
)

var statusCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "status",
	Short: "show credential source and active profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, p, tok, src, stored, err := readCredentialState()
		if err != nil {
			return err
		}

		hasCredential := tok != ""
		authenticated := usableCredential(tok, stored)

		// A device login parked by an earlier `auth login` may have been
		// authorized in the browser after the process that started it went
		// away. This is where that gets noticed: the connect and reconnect
		// flows both poll `status`, so finishing the exchange here turns a
		// killed auth subprocess from a dead end into a delay.
		var pendingAuthorization bool
		if !authenticated {
			completed, stillPending := resumePendingDeviceLogin(cmd.Context())
			if completed {
				f, p, tok, src, stored, err = readCredentialState()
				if err != nil {
					return err
				}
				hasCredential = tok != ""
				authenticated = usableCredential(tok, stored)
			} else {
				pendingAuthorization = stillPending
			}
		}

		// Ask the server whether this credential still works.
		//
		// The stored token's own clock cannot answer that. A credential is
		// dead the moment the server stops accepting it — the signing key was
		// rotated, the session was revoked, the account was disabled — and
		// none of those leave a mark on the copy in the keychain, which goes
		// on looking valid for the rest of its nominal lifetime. Reporting
		// from the local expiry alone told a connector host it was connected
		// while every request it made failed, with nothing prompting the user
		// to reconnect. This is also what the connector spec asks for (§12,
		// 方案 B: status 命令执行时从服务端拉取 token).
		//
		// The provider is the profile's real one, not a static token: a
		// two-hour-old session is supposed to refresh here exactly as it
		// would under any other command, and a static token would make that
		// ordinary case look like a rejection.
		//
		// Bounded well under the ten seconds a connector host allows for a
		// status check: the shared client's 30s timeout would blow that
		// budget on a stalled network.
		var nickname, email string
		if authenticated && f.Current != "" {
			if url, urlErr := endpoints.Resolve(p, "account"); urlErr == nil {
				ctx, cancel := context.WithTimeout(cmd.Context(), whoamiTimeout)
				tp := cliauth.TokenProviderFor(p)
				if tp == nil {
					tp = httpclient.StaticToken(tok)
				}
				u, whoamiErr := account.New(url, tp).Whoami(ctx)
				cancel()
				switch {
				case whoamiErr == nil:
					nickname = u.Nickname
					email = u.Email
				case cliauth.IsSessionDead(whoamiErr):
					// Permanently rejected. The provider has already purged
					// the credential on its way to this error, so re-read the
					// state: reporting the token_status of a credential that
					// no longer exists would be the same lie in a new place.
					authenticated = false
					if f2, p2, tok2, src2, stored2, rerr := readCredentialState(); rerr == nil {
						f, p, tok, src, stored = f2, p2, tok2, src2, stored2
						hasCredential = tok != ""
					}
				default:
					// Unreachable, timed out, 5xx. We learned nothing, so we
					// say nothing: the local verdict stands rather than
					// flapping a connection that is probably fine.
				}
			}
		}
		if !hasCredential {
			src = "none"
		}

		authMethod := "pat"
		if stored.TokenType == "oauth" {
			authMethod = "device_code"
		}

		tokenStatus := "unknown"
		if hasCredential {
			switch stored.Status() {
			case credential.StatusValid:
				tokenStatus = "valid"
			case credential.StatusNeedsRefresh:
				tokenStatus = "needs_refresh"
			case credential.StatusExpired:
				tokenStatus = "expired"
			}
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			payload := map[string]any{
				"authenticated": authenticated,
				"profile":       f.Current,
				"source":        src,
			}
			if !authenticated {
				payload["hint"] = i18n.T("auth.status.json.hint")
				// An expired credential is worth naming: "not signed in"
				// and "signed in but the session died" call for the same
				// fix (re-login) but are different facts.
				if hasCredential {
					payload["token_status"] = tokenStatus
				}
				// "A login is open and waiting on the user" is a third
				// fact, and the one that changes what a caller should do:
				// wait and re-check, rather than start another login.
				if pendingAuthorization {
					payload["pending_authorization"] = true
					payload["hint"] = i18n.T("auth.status.json.hint_pending")
				}
			} else {
				if p.CredentialRef != "" {
					payload["credential_ref"] = p.CredentialRef
				}
				payload["auth_method"] = authMethod
				payload["token_status"] = tokenStatus
				if !stored.ExpiresAt.IsZero() {
					payload["expires_at"] = stored.ExpiresAt.UTC().Format(time.RFC3339)
				}
				if !stored.RefreshExpiresAt.IsZero() {
					payload["refresh_expires_at"] = stored.RefreshExpiresAt.UTC().Format(time.RFC3339)
				}
				if nickname != "" || email != "" {
					user := map[string]any{}
					if nickname != "" {
						user["nickname"] = nickname
					}
					if email != "" {
						user["email"] = email
					}
					payload["user"] = user
				}
			}
			if err := output.NewJSON(cmd.OutOrStdout()).Object(payload); err != nil {
				return err
			}
			return exitForAuthState(authenticated)
		}

		// --- default: text mode (unchanged behavior) ---
		if !authenticated {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.not_logged_in"))
			if pendingAuthorization {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+i18n.T("auth.status.json.hint_pending"))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+i18n.T("auth.status.json.hint"))
			}
			return exitForAuthState(false)
		}

		var userInfo string
		if email != "" {
			userInfo = fmt.Sprintf("%s (%s)", nickname, email)
		} else {
			userInfo = nickname
		}
		if userInfo != "" {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.signed_in_as", userInfo))
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.signed_in"))
		}

		textAuthMethod := "PAT"
		if authMethod == "device_code" {
			textAuthMethod = "Device Code Flow"
		}
		fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.method", textAuthMethod))

		if src == "env" {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.source.env"))
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.source.keychain", p.CredentialRef))
		}

		switch tokenStatus {
		case "valid", "needs_refresh":
			// Both states are "session usable" from the user's POV.
			// needs_refresh just means the short-lived access token is
			// near/past expiry, but it gets refreshed transparently on the
			// next API call; the user does not need to act.
			//
			// Show session (refresh token) expiry, not access token expiry —
			// the ~2h access lifetime is not meaningful to users. Fall back
			// to access expiry for legacy tokens missing refresh_expires_at.
			switch {
			case !stored.RefreshExpiresAt.IsZero():
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.token.valid", formatDuration(time.Until(stored.RefreshExpiresAt))))
			case !stored.ExpiresAt.IsZero():
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.token.valid", formatDuration(time.Until(stored.ExpiresAt))))
			default:
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.token.forever"))
			}
		case "expired":
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.token.expired"))
		}

		fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.status.field.profile", orNone(f.Current)))
		return nil
	},
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return i18n.T("auth.duration.m", 0)
	}
	days := int(d / (24 * time.Hour))
	if days > 0 {
		return i18n.T("auth.duration.d", days)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return i18n.T("auth.duration.hm", h, m)
	}
	return i18n.T("auth.duration.m", m)
}

// whoamiTimeout bounds the server round-trip `auth status` makes to check the
// credential is still accepted. It has to cover more than one request — a
// stale access token is refreshed and the call retried — while leaving room
// inside the ten seconds a connector host allows a status check.
//
// Overrunning it is not a failure verdict: an unanswered call means the
// server's opinion is unknown, and status falls back to the local one. That is
// why the budget can be this tight.
const whoamiTimeout = 5 * time.Second

// readCredentialState loads the active profile and whatever credential it
// resolves to. It exists as a function because `auth status` reads this
// twice: once to decide, and again after a parked device login completes
// and the keychain has changed underneath it.
func readCredentialState() (config.ProfilesFile, config.Profile, string, string, credential.StoredToken, error) {
	f, err := config.LoadProfiles()
	if err != nil {
		return config.ProfilesFile{}, config.Profile{}, "", "", credential.StoredToken{}, err
	}

	var p config.Profile
	if f.Current != "" {
		if prof, profErr := cliauth.CurrentProfile(); profErr == nil {
			p = prof
		}
	}

	// Resolve token: env wins, else keychain.
	if envTok := os.Getenv("VIBEKNOW_TOKEN"); envTok != "" {
		return f, p, envTok, "env", credential.NewPATToken(envTok), nil
	}
	if p.CredentialRef != "" {
		if kc, kcErr := keychain.OpenFor("vibeknow"); kcErr == nil {
			ks := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
			if st, stErr := ks.GetStored(); stErr == nil && st.AccessToken != "" {
				return f, p, st.AccessToken, "keychain", st, nil
			}
		}
	}
	return f, p, "", "", credential.StoredToken{}, nil
}

// usableCredential reports whether a stored credential can still make a
// successful call. A stored token is not the same as a usable one:
// StatusExpired means the refresh token is dead too, so nothing succeeds
// until the user logs in again. Counting expired as authenticated kept
// status consumers (WorkBuddy's statusMatchJson polls `auth status
// --output json` every 3s) showing "connected" on a dead credential until
// some business command finally failed with exit 3. needs_refresh stays
// authenticated — the next API call refreshes transparently.
func usableCredential(tok string, stored credential.StoredToken) bool {
	return tok != "" && stored.Status() != credential.StatusExpired
}

// exitForAuthState makes the process exit code say the same thing the report
// on stdout says: 0 when connected, 3 — the CLI's auth code — when not.
//
// `status` answers a question, so for a long time it exited 0 either way and
// let the payload carry the answer. That is fine for a human and wrong for a
// connector host, which decides whether to start a login from the exit code:
// the WorkBuddy spec's connect sequence goes "run status → exit code ≠ 0 →
// run auth", and its own acceptance checklist asks for "0=已认证, 非 0=未认证".
// Exiting 0 while unauthenticated invites that host to skip the login and
// show a connected card over a machine that has no credential, where every
// command then fails with this same code 3 anyway.
//
// The spec is not self-consistent here — its detailed decision table checks
// statusMatchJson after the exit code, and by that reading exiting 0 was
// survivable. Being correct under only one of two readings is not worth the
// nothing it buys, so the exit code now agrees with the payload under both.
//
// Nothing is printed for the non-zero case: the report is the message, and
// the host parses it.
func exitForAuthState(authenticated bool) error {
	if authenticated {
		return nil
	}
	return clerr.SilentExit(clerr.ExitAuth)
}
