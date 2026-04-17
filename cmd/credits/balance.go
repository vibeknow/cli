package credits

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

var balanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "show your credit balance",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newVibeknowClient()
		if err != nil {
			return err
		}

		b, err := c.GetBalance(context.Background())
		if err != nil {
			return err
		}

		fmt.Printf("可用积分: %d\n", b.TotalBalance)
		if b.TotalFrozen > 0 {
			fmt.Printf("冻结积分: %d\n", b.TotalFrozen)
		}
		return nil
	},
}

type staticToken string

func (s staticToken) Token(_ context.Context) (string, error) { return string(s), nil }

func newVibeknowClient() (*vibeknow.Client, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	tok, _, err := cliauth.ResolverFor(p).Resolve()
	if err != nil {
		return nil, fmt.Errorf("no credential available; run `vibeknow auth login`")
	}
	url, err := endpoints.Resolve(p, "vibeknow")
	if err != nil {
		return nil, err
	}
	return vibeknow.New(url, staticToken(tok)), nil
}
