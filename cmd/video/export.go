package video

import (
	"context"
	"encoding/json"
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

	ctx, cancel := signalContext(cmd.Context())
	defer cancel()

	// Submit (backend is idempotent: same (user,session) → same task_id).
	exportTaskID, err := c.ExportVideo(ctx, flagExportSessionID)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.submitted", exportTaskID))

	if flagExportAsync {
		return emitSnapshot(cmd, format, taskID, flagExportSessionID, c, exportTaskID, nil)
	}

	// Sync polling loop.
	ndjson := format == "ndjson"
	result, pollErr := exportpoll.PollExport(ctx, c, exportTaskID, flagExportTimeout, flagExportPollInterval, func(ev exportpoll.Event) {
		emitPollEvent(cmd, ndjson, exportTaskID, ev)
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
	return emitSnapshot(cmd, format, taskID, flagExportSessionID, c, exportTaskID, result)
}

func emitPollEvent(cmd *cobra.Command, ndjson bool, exportTaskID string, ev exportpoll.Event) {
	if ndjson {
		out := map[string]any{
			"type":           eventType(ev.Status),
			"export_task_id": exportTaskID,
			"progress":       ev.Progress,
		}
		if ev.ProgressMsg != "" {
			out["progress_msg"] = ev.ProgressMsg
		}
		_ = output.NewNDJSON(cmd.OutOrStdout()).Event(out)
		return
	}
	switch ev.Status {
	case snapshot.StatusSucceeded:
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.succeeded"))
	case snapshot.StatusFailed:
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.failed", ev.ProgressMsg))
	default:
		if term.IsTerminal(int(os.Stderr.Fd())) {
			if ev.ProgressMsg != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r%s", i18n.T("export.progress", ev.Progress, ev.ProgressMsg))
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r%s", i18n.T("export.progress_simple", ev.Progress))
			}
		} else if ev.Progress%10 == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("export.progress_simple", ev.Progress))
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
// chosen format. Shared with export_status.go (Task 12) via package scope.
func emitSnapshot(cmd *cobra.Command, format string, taskID int64, sessionID string, c *figlens.Client, exportTaskID string, export *figlens.ExportResult) error {
	work, err := c.GetWorkBySession(cmd.Context(), sessionID)
	if err != nil {
		return err
	}
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:       taskID,
		SessionID:    sessionID,
		Work:         work,
		Export:       export,
		ExportTaskID: exportTaskID,
		ShareBase:    cmdutil.ShareBaseURL(),
	})
	if format == "json" {
		return snapshot.RenderJSON(cmd.OutOrStdout(), s)
	}
	if format == "ndjson" {
		b, _ := json.Marshal(s)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["type"] = "snapshot"
		return output.NewNDJSON(cmd.OutOrStdout()).Event(m)
	}
	snapshot.RenderText(cmd.OutOrStdout(), cmd.ErrOrStderr(), s)
	return nil
}

// signalContext returns a context that cancels on SIGINT/SIGTERM so the sync
// poll loop exits cleanly. The backend keeps rendering regardless — users
// re-attach with vk video export-status.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sig)
	}()
	return ctx, cancel
}

func init() {
	exportCmd.Flags().StringVar(&flagExportSessionID, "session-id", "", "session ID (required)")
	exportCmd.Flags().BoolVar(&flagExportAsync, "async", false, "submit and return; do not wait")
	exportCmd.Flags().BoolVarP(&flagExportYes, "yes", "y", false, "skip confirmation prompt")
	exportCmd.Flags().DurationVar(&flagExportTimeout, "timeout", exportDefaultTimeout(), "sync-mode deadline")
	exportCmd.Flags().DurationVar(&flagExportPollInterval, "poll-interval", 0, "fixed poll interval (overrides exponential backoff)")
}

func exportDefaultTimeout() time.Duration {
	if v := os.Getenv("VIBEKNOW_EXPORT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 15 * time.Minute
}
