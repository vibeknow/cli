package video

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var flagExportStatusSessionID string

var exportStatusCmd = &cobra.Command{
	Use:   "export-status <export_task_id>",
	Short: "poll a specific export task by its export_task_id",
	Args:  cobra.ExactArgs(1),
	Example: `  vk video export-status 424242 --session-id sess_xxx
  vk video export-status 424242 --session-id sess_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		exportTaskID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return clerr.Validationf("export_task_id must be an integer: %v", err)
		}
		// The positional argument here is an export_task_id, not a task_id,
		// so it is no help in finding the session — only the flag or the
		// most recent recorded run can supply it.
		_, sessionID, err := resolveTarget(cmd.Context(), nil, flagExportStatusSessionID)
		if err != nil {
			return err
		}
		flagExportStatusSessionID = sessionID

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
		//
		// This is a single-shot query, so a failed export is reported in the
		// payload (export.status="failed") and the command still exits 0 —
		// it succeeded at answering. Only the blocking form, `vk video
		// export` without --async, adopts the export's terminal state as its
		// own exit code.
		_, err = emitSnapshot(cmd, format, snapshot.BuildInput{
			SessionID:    flagExportStatusSessionID,
			Export:       result,
			ExportTaskID: exportTaskID,
		}, c)
		return err
	},
}

func init() {
	exportStatusCmd.Flags().StringVar(&flagExportStatusSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
}
