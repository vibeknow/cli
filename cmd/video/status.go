package video

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var flagStatusSessionID string

var statusCmd = &cobra.Command{
	Use:   "status [task_id]",
	Short: "full snapshot: preview state + export state + next_actions",
	Args:  cobra.MaximumNArgs(1),
	Example: `  vk video status --session-id sess_xxx
  vk video status 123 --session-id sess_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagStatusSessionID == "" {
			return clerr.Validation("--session-id is required")
		}
		var taskID int64
		if len(args) == 1 {
			parsed, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return clerr.Validationf("task_id must be an integer: %v", err)
			}
			taskID = parsed
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		work, err := c.GetWorkBySession(context.Background(), flagStatusSessionID)
		if err != nil {
			return err
		}

		s := snapshot.Build(snapshot.BuildInput{
			TaskID:    taskID,
			SessionID: flagStatusSessionID,
			Work:      work,
			ShareBase: cmdutil.ShareBaseURL(),
		})

		format, _ := cmd.Flags().GetString("output")
		return snapshot.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), s, format)
	},
}

func init() {
	statusCmd.Flags().StringVar(&flagStatusSessionID, "session-id", "", "session ID (required)")
}
