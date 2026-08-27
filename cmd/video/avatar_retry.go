package video

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var (
	flagAvatarRetrySessionID string
	flagAvatarRetryScene     int
)

var avatarRetryCmd = &cobra.Command{
	Use:   "avatar-retry [task_id]",
	Short: "retry a work's failed avatar scenes (no re-charge)",
	Args:  cobra.MaximumNArgs(1),
	Long: `Failed avatar scenes are terminal: the MP4 export gate refuses to render
a video with blank presenter windows, so a work with any failed scene can
never export until those scenes are retried. This re-runs the avatar
render for exactly those scenes — the script, images, TTS and every
healthy scene are untouched, and nothing is billed again.`,
	Example: `  vk video avatar-retry
  vk video avatar-retry 42
  vk video avatar-retry 42 --scene 3`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagAvatarRetrySessionID)
		if err != nil {
			return err
		}
		_ = taskID // addressing is by session; the arg exists for ledger lookup symmetry

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		var scene *int
		if cmd.Flags().Changed("scene") {
			scene = &flagAvatarRetryScene
		}
		count, err := c.RetryAvatarScenes(cmd.Context(), sessionID, scene)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" || format == "ndjson" {
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"session_id":  sessionID,
				"retry_count": count,
				"next_actions": []map[string]string{{
					"command": fmt.Sprintf("vk video export %s", snapshot.Target(taskID, sessionID)),
					"purpose": "Export the MP4 once the retried scenes finish",
				}},
			})
		}
		if count == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("avatar.retry.nothing"))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), i18n.T("avatar.retry.result", count))
		return nil
	},
}

func init() {
	avatarRetryCmd.Flags().StringVar(&flagAvatarRetrySessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	avatarRetryCmd.Flags().IntVar(&flagAvatarRetryScene, "scene", -1, "retry only this scene index (default: every failed scene)")
}
