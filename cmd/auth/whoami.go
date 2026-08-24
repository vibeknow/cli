package auth

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
)

var whoamiCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "whoami",
	Short: "print the current authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		tok, _, err := cliauth.ResolverFor(p).Resolve()
		if err != nil {
			return clerr.Auth(i18n.T("auth.not_logged_in")).WithHint(i18n.T("auth.not_logged_in.hint"))
		}
		url, err := endpoints.Resolve(p, "account")
		if err != nil {
			return err
		}
		c := account.New(url, httpclient.StaticToken(tok))
		u, err := c.Whoami(context.Background())
		if err != nil {
			return err
		}
		return cmdutil.Emit(cmd, map[string]any{
			"uid":      u.UID,
			"nickname": u.Nickname,
			"email":    u.Email,
			"phone":    u.Phone,
			"profile":  p.Name,
		}, "auth.whoami", func(w io.Writer) {
			fmt.Fprintf(w, "uid: %d\nnickname: %s\nemail: %s\nphone: %s\n", u.UID, u.Nickname, u.Email, u.Phone)
		})
	},
}
