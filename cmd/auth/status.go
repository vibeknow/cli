package auth

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/account"
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
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}

		// Determine active profile info.
		var p config.Profile
		if f.Current != "" {
			if prof, err := cliauth.CurrentProfile(); err == nil {
				p = prof
			}
		}

		// Resolve token: env wins, else keychain.
		var tok string
		var src string
		var stored credential.StoredToken

		if envTok := os.Getenv("VIBEKNOW_TOKEN"); envTok != "" {
			tok = envTok
			src = "env"
			stored = credential.NewPATToken(envTok)
		} else if p.CredentialRef != "" {
			if kc, kcErr := keychain.OpenFor("vibeknow"); kcErr == nil {
				ks := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
				if st, stErr := ks.GetStored(); stErr == nil && st.AccessToken != "" {
					tok = st.AccessToken
					src = "keychain"
					stored = st
				}
			}
		}

		authenticated := tok != ""
		if !authenticated {
			src = "none"
		}

		// Optional whoami (don't fail on network error).
		var nickname, email string
		if authenticated && f.Current != "" {
			if url, urlErr := endpoints.Resolve(p, "account"); urlErr == nil {
				c := account.New(url, httpclient.StaticToken(tok))
				if u, whoamiErr := c.Whoami(context.Background()); whoamiErr == nil {
					nickname = u.Nickname
					email = u.Email
				}
			}
		}

		authMethod := "pat"
		if stored.TokenType == "oauth" {
			authMethod = "device_code"
		}

		tokenStatus := "unknown"
		if authenticated {
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
			return output.NewJSON(cmd.OutOrStdout()).Object(payload)
		}

		// --- default: text mode (unchanged behavior) ---
		if !authenticated {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("auth.not_logged_in"))
			fmt.Fprintln(cmd.OutOrStdout(), "  "+i18n.T("auth.status.json.hint"))
			return nil
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
