package kb

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/i18n"
)

var flagDeleteYes bool

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: i18n.T("kb.delete.short"),
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbID := args[0]

		ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
			Prompt: i18n.T("kb.delete.confirm.no_name", kbID),
			Yes:    flagDeleteYes,
		})
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		vc, err := cliauth.NewVectoriaClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := vc.DeleteKB(ctx, kbID); err != nil {
			// 404 → idempotent success (rm -f semantics).
			if errs.HasCode(err, "not_found") {
				fmt.Println(i18n.T("kb.delete.already_gone"))
				return nil
			}
			return err
		}
		fmt.Println(i18n.T("kb.delete.done"))
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVarP(&flagDeleteYes, "yes", "y", false, "skip confirmation prompt")
	Cmd.AddCommand(deleteCmd)
}
