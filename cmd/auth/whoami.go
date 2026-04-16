package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/client/account"
	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "print the current authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		tok, _, err := cliauth.ResolverFor(p).Resolve()
		if err != nil {
			return fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
		}
		url, err := endpoints.Resolve(p, "account")
		if err != nil {
			return err
		}
		c := account.New(url, staticToken(tok))
		u, err := c.Whoami(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("uid: %d\nnickname: %s\nemail: %s\nphone: %s\n", u.UID, u.Nickname, u.Email, u.Phone)
		return nil
	},
}

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }
