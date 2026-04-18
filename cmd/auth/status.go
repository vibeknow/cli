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
	"github.com/vibeknow/cli/internal/keychain"
	"github.com/vibeknow/cli/internal/output"
)

var statusCmd = &cobra.Command{
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
				c := account.New(url, staticToken(tok))
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
				payload["hint"] = "运行 `vibeknow auth login` 或设置 VIBEKNOW_TOKEN 环境变量"
			} else {
				if p.CredentialRef != "" {
					payload["credential_ref"] = p.CredentialRef
				}
				payload["auth_method"] = authMethod
				payload["token_status"] = tokenStatus
				if !stored.ExpiresAt.IsZero() {
					payload["expires_at"] = stored.ExpiresAt.UTC().Format(time.RFC3339)
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
			fmt.Fprintln(cmd.OutOrStdout(), "未登录")
			fmt.Fprintln(cmd.OutOrStdout(), "  运行 `vibeknow auth login` 或设置 VIBEKNOW_TOKEN 环境变量")
			return nil
		}

		var userInfo string
		if email != "" {
			userInfo = fmt.Sprintf("%s (%s)", nickname, email)
		} else {
			userInfo = nickname
		}
		if userInfo != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ 已登录为 %s\n", userInfo)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "✓ 已登录")
		}

		textAuthMethod := "PAT"
		if authMethod == "device_code" {
			textAuthMethod = "Device Code Flow"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  - 认证方式: %s\n", textAuthMethod)

		if src == "env" {
			fmt.Fprintln(cmd.OutOrStdout(), "  - Token 来源: 环境变量 (VIBEKNOW_TOKEN)")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  - Token 来源: 系统密钥链 (%s)\n", p.CredentialRef)
		}

		switch tokenStatus {
		case "valid":
			if !stored.ExpiresAt.IsZero() {
				fmt.Fprintf(cmd.OutOrStdout(), "  - Token 状态: 有效 (%s后过期)\n", formatDuration(time.Until(stored.ExpiresAt)))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "  - Token 状态: 有效 (永不过期)")
			}
		case "needs_refresh":
			fmt.Fprintln(cmd.OutOrStdout(), "  - Token 状态: 需要刷新")
		case "expired":
			fmt.Fprintln(cmd.OutOrStdout(), "  - Token 状态: 已过期")
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  - Active profile: %s\n", orNone(f.Current))
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
		return "0分"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分", h, m)
	}
	return fmt.Sprintf("%d分", m)
}
