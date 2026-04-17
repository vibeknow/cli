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
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "show credential source and active profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}

		// Determine active profile info
		var p config.Profile
		if f.Current != "" {
			if prof, err := cliauth.CurrentProfile(); err == nil {
				p = prof
			}
		}

		// Step 1: Check env var first
		var tok string
		var src string
		var stored credential.StoredToken

		if envTok := os.Getenv("VIBEKNOW_TOKEN"); envTok != "" {
			tok = envTok
			src = "env"
			stored = credential.NewPATToken(envTok)
		} else if p.CredentialRef != "" {
			// Step 2: Check keychain via GetStored
			if kc, kcErr := keychain.OpenFor("vibeknow"); kcErr == nil {
				ks := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
				if st, stErr := ks.GetStored(); stErr == nil && st.AccessToken != "" {
					tok = st.AccessToken
					src = "keychain"
					stored = st
				}
			}
		}

		// Step 3: If no token found
		if tok == "" {
			fmt.Println("未登录")
			fmt.Println("  运行 `vibeknow auth login` 或设置 VIBEKNOW_TOKEN 环境变量")
			return nil
		}

		// Step 4: Try whoami (optional, don't fail on network error)
		var userInfo string
		if f.Current != "" {
			if url, urlErr := endpoints.Resolve(p, "account"); urlErr == nil {
				c := account.New(url, staticToken(tok))
				if u, whoamiErr := c.Whoami(context.Background()); whoamiErr == nil {
					if u.Email != "" {
						userInfo = fmt.Sprintf("%s (%s)", u.Nickname, u.Email)
					} else {
						userInfo = u.Nickname
					}
				}
			}
		}

		if userInfo != "" {
			fmt.Printf("✓ 已登录为 %s\n", userInfo)
		} else {
			fmt.Println("✓ 已登录")
		}

		// Auth method
		authMethod := "PAT"
		if stored.TokenType == "oauth" {
			authMethod = "Device Code Flow"
		}
		fmt.Printf("  - 认证方式: %s\n", authMethod)

		// Token source
		if src == "env" {
			fmt.Println("  - Token 来源: 环境变量 (VIBEKNOW_TOKEN)")
		} else {
			fmt.Printf("  - Token 来源: 系统密钥链 (%s)\n", p.CredentialRef)
		}

		// Token status
		status := stored.Status()
		switch status {
		case credential.StatusValid:
			if !stored.ExpiresAt.IsZero() {
				remaining := time.Until(stored.ExpiresAt)
				fmt.Printf("  - Token 状态: 有效 (%s后过期)\n", formatDuration(remaining))
			} else {
				fmt.Println("  - Token 状态: 有效 (永不过期)")
			}
		case credential.StatusNeedsRefresh:
			fmt.Println("  - Token 状态: 需要刷新")
		case credential.StatusExpired:
			fmt.Println("  - Token 状态: 已过期")
		}

		// Active profile
		fmt.Printf("  - Active profile: %s\n", orNone(f.Current))

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
