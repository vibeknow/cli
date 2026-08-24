package credits

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
)

var balanceCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
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

		// The text form is localized prose, so it was unparseable by
		// definition — a caller checking "do I have enough credits before
		// spending several minutes on a render" had nothing to read.
		return cmdutil.Emit(cmd, map[string]any{
			"balance":   b.TotalBalance,
			"frozen":    b.TotalFrozen,
			"available": b.TotalBalance,
		}, "credits.balance", func(w io.Writer) {
			fmt.Fprintln(w, i18n.T("credits.available", b.TotalBalance))
			if b.TotalFrozen > 0 {
				fmt.Fprintln(w, i18n.T("credits.frozen", b.TotalFrozen))
			}
		})
	},
}

func newVibeknowClient() (*vibeknow.Client, error) {
	_, url, tp, err := cmdutil.Default().Service("vibeknow")
	if err != nil {
		return nil, err
	}
	return vibeknow.New(url, tp), nil
}
