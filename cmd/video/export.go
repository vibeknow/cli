package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/exportpoll"
	"github.com/vibeknow/cli/internal/video/snapshot"
)


var (
	flagExportSessionID    string
	flagExportAsync        bool
	flagExportYes          bool
	flagExportTimeout      time.Duration
	flagExportPollInterval time.Duration
)

var exportCmd = &cobra.Command{
	Use:   "export <task_id>",
	Short: "render the MP4 for a work (~several minutes, extra credits)",
	Args:  cobra.ExactArgs(1),
	Example: `  vk video export 123 --session-id sess_xxx
  vk video export 123 --session-id sess_xxx --async
  vk video export 123 --session-id sess_xxx --yes --timeout 20m`,
	RunE: runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	if flagExportSessionID == "" {
		return clerr.Validation("--session-id is required")
	}
	taskID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return clerr.Validationf("task_id must be an integer: %v", err)
	}
	c, err := newFiglensClient()
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("output")

	// Confirmation gate (paid operation).
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: i18n.T("export.confirm_prompt"),
		Yes:    flagExportYes,
	})
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.cancelled"))
		return nil
	}

	ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Submit (backend is idempotent: same (user,session) → same task_id).
	exportTaskID, err := c.ExportVideo(ctx, flagExportSessionID)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.submitted", exportTaskID))

	if flagExportAsync {
		return emitSnapshot(cmd, format, snapshot.BuildInput{
			TaskID:       taskID,
			SessionID:    flagExportSessionID,
			ExportTaskID: exportTaskID,
		}, c)
	}

	// Sync polling loop.
	progressTTY := term.IsTerminal(int(os.Stderr.Fd()))
	var emitNDJSON func(map[string]any) error
	if format == "ndjson" {
		w := output.NewNDJSON(cmd.OutOrStdout())
		emitNDJSON = w.Event
	}
	lastProgress, lastMsg := -1, ""
	result, pollErr := exportpoll.PollExport(ctx, c, exportTaskID, flagExportTimeout, flagExportPollInterval, func(ev exportpoll.Event) {
		// Skip duplicate running-state ticks so NDJSON streams and
		// non-TTY logs don't spam the same progress line. Stage
		// transitions (ProgressMsg change) still emit even at the same %.
		if ev.Status == snapshot.StatusRunning && ev.Progress == lastProgress && ev.ProgressMsg == lastMsg {
			return
		}
		lastProgress, lastMsg = ev.Progress, ev.ProgressMsg
		emitPollEvent(cmd, emitNDJSON, progressTTY, exportTaskID, ev)
	})
	if errors.Is(pollErr, context.Canceled) {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.sigint_detach", exportTaskID, flagExportSessionID))
		os.Exit(6)
	}
	if errors.Is(pollErr, exportpoll.ErrTimeout) {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.timeout", flagExportTimeout))
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.reattach_hint", exportTaskID, flagExportSessionID))
		os.Exit(6)
	}
	if pollErr != nil {
		return pollErr
	}
	return emitSnapshot(cmd, format, snapshot.BuildInput{
		TaskID:       taskID,
		SessionID:    flagExportSessionID,
		Export:       result,
		ExportTaskID: exportTaskID,
	}, c)
}

func emitPollEvent(cmd *cobra.Command, emitNDJSON func(map[string]any) error, progressTTY bool, exportTaskID string, ev exportpoll.Event) {
	if emitNDJSON != nil {
		out := map[string]any{
			"type":           eventType(ev.Status),
			"export_task_id": exportTaskID,
			"progress":       ev.Progress,
		}
		if ev.ProgressMsg != "" {
			out["progress_msg"] = ev.ProgressMsg
		}
		_ = emitNDJSON(out)
		return
	}
	stderr := cmd.ErrOrStderr()
	switch ev.Status {
	case snapshot.StatusSucceeded:
		fmt.Fprintln(stderr, i18n.T("export.succeeded"))
	case snapshot.StatusFailed:
		fmt.Fprintln(stderr, i18n.T("export.failed", ev.ProgressMsg))
	default:
		if progressTTY {
			if ev.ProgressMsg != "" {
				fmt.Fprintf(stderr, "\r%s", i18n.T("export.progress", ev.Progress, ev.ProgressMsg))
			} else {
				fmt.Fprintf(stderr, "\r%s", i18n.T("export.progress_simple", ev.Progress))
			}
		} else if ev.Progress%10 == 0 {
			fmt.Fprintln(stderr, i18n.T("export.progress_simple", ev.Progress))
		}
	}
}

func eventType(status string) string {
	switch status {
	case snapshot.StatusSucceeded:
		return "export.succeeded"
	case snapshot.StatusFailed:
		return "export.failed"
	default:
		return "export.progress"
	}
}

// emitSnapshot fetches the work row and renders the snapshot in the user's
// chosen format. Shared with export_status.go via package scope.
func emitSnapshot(cmd *cobra.Command, format string, in snapshot.BuildInput, c *figlens.Client) error {
	work, err := c.GetWorkBySession(cmd.Context(), in.SessionID)
	if err != nil {
		return err
	}
	in.Work = work
	in.ShareBase = cmdutil.ShareBaseURL()
	s := snapshot.Build(in)
	return snapshot.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), s, format)
}

func init() {
	exportCmd.Flags().StringVar(&flagExportSessionID, "session-id", "", "session ID (required)")
	exportCmd.Flags().BoolVar(&flagExportAsync, "async", false, "submit and return; do not wait")
	exportCmd.Flags().BoolVarP(&flagExportYes, "yes", "y", false, "skip confirmation prompt")
	exportCmd.Flags().DurationVar(&flagExportTimeout, "timeout", exportpoll.DefaultTimeout(), "sync-mode deadline")
	exportCmd.Flags().DurationVar(&flagExportPollInterval, "poll-interval", 0, "fixed poll interval (overrides exponential backoff)")
}
