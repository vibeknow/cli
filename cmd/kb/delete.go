package kb

import (
	"context"
	"fmt"
	"io"
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

		// The vectoria backend does not expose GET /v1/knowledgebases/<id>
		// (returns 405), so we can't enrich the prompt with the kb's name
		// without an O(N) list-and-scan. The "no_name" variant is the only
		// path; in practice users got the id from `vk kb list` and already
		// know which one they're deleting.
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
		alreadyGone := false
		if err := vc.DeleteKB(ctx, kbID); err != nil {
			// 404 → idempotent success (rm -f semantics).
			if !errs.HasCode(err, "not_found") {
				return err
			}
			alreadyGone = true
		}
		// already_gone rides in the payload rather than in the exit code:
		// the outcome the caller asked for ("this kb should not exist")
		// holds either way, but a caller reconciling state wants to know
		// whether this run is what made it true.
		return cmdutil.Emit(cmd, map[string]any{
			"kb_id":        kbID,
			"deleted":      !alreadyGone,
			"already_gone": alreadyGone,
		}, "kb.deleted", func(w io.Writer) {
			if alreadyGone {
				fmt.Fprintln(w, i18n.T("kb.delete.already_gone"))
				return
			}
			fmt.Fprintln(w, i18n.T("kb.delete.done"))
		})
	},
}

func init() {
	deleteCmd.Flags().BoolVarP(&flagDeleteYes, "yes", "y", false, "skip confirmation prompt")
	Cmd.AddCommand(deleteCmd)
}
