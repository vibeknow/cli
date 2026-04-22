package video

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var flagExportStatusSessionID string

var exportStatusCmd = &cobra.Command{
	Use:   "export-status <export_task_id>",
	Short: "poll a specific export task by its export_task_id",
	Args:  cobra.ExactArgs(1),
	Example: `  vk video export-status exp_1 --session-id sess_xxx
  vk video export-status exp_1 --session-id sess_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagExportStatusSessionID == "" {
			return clerr.Validation("--session-id is required")
		}
		exportTaskID := args[0]

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		ctx := context.Background()

		result, err := c.GetExportResult(ctx, exportTaskID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		// task_id is not recoverable from export_task_id alone; pass 0.
		// Agents that care have it from the original `vk create` response.
		return emitSnapshot(cmd, format, snapshot.BuildInput{
			SessionID:    flagExportStatusSessionID,
			Export:       result,
			ExportTaskID: exportTaskID,
		}, c)
	},
}

func init() {
	exportStatusCmd.Flags().StringVar(&flagExportStatusSessionID, "session-id", "", "session ID (required)")
}
