package jobs

import (
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/jobs"
)

var getCmd = &cobra.Command{
	Use:   "get [task_id]",
	Short: "show one recorded run (default: the most recent)",
	Args:  cobra.MaximumNArgs(1),
	Example: `  vk jobs get
  vk jobs get 42 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			r     jobs.Record
			found bool
			err   error
		)
		if len(args) == 1 {
			taskID, perr := strconv.ParseInt(args[0], 10, 64)
			if perr != nil {
				return clerr.Validationf("task_id must be an integer: %v", perr)
			}
			r, found, err = jobs.Get(taskID)
		} else {
			r, found, err = jobs.Latest()
		}
		if err != nil {
			return err
		}
		if !found {
			// Exit 2, not 1: nothing is broken, the caller asked for
			// something that is not in the ledger and the fix is an
			// argument change.
			return clerr.Validation("no matching run in the local ledger").
				WithHint("`vk jobs list` shows what is recorded; the ledger only covers runs started by this machine's `vk create`")
		}
		return cmdutil.Emit(cmd, recordMap(r), "jobs.get", func(w io.Writer) {
			fmt.Fprintf(w, "task_id=%d session_id=%s\n", r.TaskID, r.SessionID)
			fmt.Fprintf(w, "status=%s age=%s\n", r.Status, age(r.UpdatedAt))
			if r.Source != "" {
				fmt.Fprintf(w, "source=%s\n", r.Source)
			}
			if r.Mode != "" {
				fmt.Fprintf(w, "mode=%s\n", r.Mode)
			}
			if r.Title != "" {
				fmt.Fprintf(w, "title=%s\n", r.Title)
			}
			if r.ShareURL != "" {
				fmt.Fprintf(w, "share_url=%s\n", r.ShareURL)
			}
			if r.VideoPath != "" {
				fmt.Fprintf(w, "video_path=%s\n", r.VideoPath)
			}
			if r.Error != "" {
				fmt.Fprintf(w, "error=%s\n", r.Error)
			}
			if !r.Terminal() {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: reattach with `vk video wait %d`\n", r.TaskID)
			}
		})
	},
}
