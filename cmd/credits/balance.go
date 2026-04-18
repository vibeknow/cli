package credits

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cmdutil"
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

func newVibeknowClient() (*vibeknow.Client, error) {
	_, url, tp, err := cmdutil.Default().Service("vibeknow")
	if err != nil {
		return nil, err
	}
	return vibeknow.New(url, tp), nil
}
