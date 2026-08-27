package video

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var (
	flagStatusSessionID  string
	flagStatusPreviewDir string
)

var statusCmd = &cobra.Command{
	Use:   "status [task_id]",
	Short: "full snapshot: preview state + export state + next_actions",
	Args:  cobra.MaximumNArgs(1),
	Example: `  vk video status
  vk video status 123
  vk video status 123 --session-id sess_xxx --output json
  vk video status 123 --preview-dir ./out --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagStatusSessionID)
		if err != nil {
			return err
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		// Built before the request so a --preview-dir that cannot be written
		// is refused without a round trip, the same way `create` refuses it.
		ch, err := cmdutil.NewRunChannel(cmd, flagStatusPreviewDir)
		if err != nil {
			return clerr.Validation(err.Error())
		}

		ctx := context.Background()
		work, err := c.GetWorkBySession(ctx, sessionID)
		if err != nil {
			return err
		}

		s := snapshot.Build(snapshot.BuildInput{
			TaskID:    taskID,
			SessionID: sessionID,
			Work:      work,
			ShareBase: cmdutil.ShareBaseURL(),
		})

		// The cover exists from the moment the preview does, and the MP4 from
		// the moment an export has produced one, so a snapshot is exactly
		// where a caller learns there is a new file worth fetching. Without
		// this, the only way to get either onto disk was to be the process
		// that started the run — an agent that reattached to someone else's
		// task, or came back after its own context was discarded, could name
		// the artifacts but never hand them over.
		//
		// Both are skipped when absent, so a status taken mid-render still
		// costs nothing and delivers nothing.
		cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, c, work)

		format, _ := cmd.Flags().GetString("output")
		return snapshot.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), s, format)
	},
}

func init() {
	statusCmd.Flags().StringVar(&flagStatusSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	statusCmd.Flags().StringVar(&flagStatusPreviewDir, "preview-dir", "", i18n.T("create.flag.preview_dir"))
}
