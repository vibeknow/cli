package video

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var flagStatusSessionID string

var statusCmd = &cobra.Command{
	Use:   "status [task_id]",
	Short: "full snapshot: preview state + export state + next_actions",
	Args:  cobra.MaximumNArgs(1),
	Example: `  vk video status
  vk video status 123
  vk video status 123 --session-id sess_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(args, flagStatusSessionID)
		if err != nil {
			return err
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		work, err := c.GetWorkBySession(context.Background(), sessionID)
		if err != nil {
			return err
		}

		s := snapshot.Build(snapshot.BuildInput{
			TaskID:    taskID,
			SessionID: sessionID,
			Work:      work,
			ShareBase: cmdutil.ShareBaseURL(),
		})

		format, _ := cmd.Flags().GetString("output")
		return snapshot.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), s, format)
	},
}

func init() {
	statusCmd.Flags().StringVar(&flagStatusSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
}
