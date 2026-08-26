package video

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var (
	flagWaitSessionID  string
	flagWaitPreviewDir string
)

var waitCmd = &cobra.Command{
	Use:   "wait [task_id]",
	Short: "stream progress for a video task, block until done",
	Args:  cobra.MaximumNArgs(1),
	Example: `  vk video wait
  vk video wait 123
  vk video wait 123 --session-id sess_xxx --output ndjson`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagWaitSessionID)
		if err != nil {
			return err
		}
		flagWaitSessionID = sessionID

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		format, _ := cmd.Flags().GetString("output")
		stdout := cmd.OutOrStdout()
		stderr := cmd.ErrOrStderr()

		ch, err := cmdutil.NewRunChannel(cmd, flagWaitPreviewDir)
		if err != nil {
			return clerr.Validation(err.Error())
		}

		var emitEvent func(map[string]any) error
		if format == "ndjson" {
			w := output.NewNDJSON(stdout)
			emitEvent = w.Event
		}

		var taskFailed, taskSucceeded, taskPaused, sawAnyEvent bool
		var successSessionID string

		var failedCode string
		var failedRetryable bool

		emit := func(ev figlens.StreamEvent) {
			sawAnyEvent = true

			switch {
			case emitEvent != nil:
				_ = emitEvent(ev.NDJSONFields())
			case ch.Structured():
				ch.Emit(ev.NDJSONFields())
			default:
				switch ev.Type {
				case "node.started":
					fmt.Fprintf(stderr, "[%s] started\n", ev.Node)
				case "node.succeeded":
					fmt.Fprintf(stderr, "[%s] done\n", ev.Node)
				case "node.warning":
					fmt.Fprintf(stderr, "[%s] warning: %s\n", ev.Node, ev.Message)
				case "node.failed":
					fmt.Fprintf(stderr, "[%s] failed: %s\n", ev.Node, ev.Message)
				case "node.progress":
					fmt.Fprintf(stderr, "[agent] %s\n", ev.Message)
				case "task.succeeded":
					fmt.Fprintf(stderr, "task succeeded\n")
				case "task.failed":
					fmt.Fprintf(stderr, "task failed: %s\n", ev.Message)
				case "task.paused":
					fmt.Fprintf(stderr, "task paused\n")
				}
			}

			switch ev.Type {
			case "task.succeeded":
				taskSucceeded = true
				successSessionID = ev.SessionID
				if successSessionID == "" {
					successSessionID = flagWaitSessionID
				}
			case "task.failed":
				taskFailed = true
				failedCode = ev.Code
				failedRetryable = ev.Retryable
			case "task.paused":
				taskPaused = true
			}
		}

		// Human path only: reassure through silent pipeline stretches
		// (the hand-drawn line emits nothing for its whole middle section).
		if emitEvent == nil && !ch.Structured() {
			stall := cmdutil.StartStallNotifier(time.Minute, func(elapsed time.Duration) {
				fmt.Fprintln(stderr, i18n.T("create.still_running", elapsed))
			})
			defer stall.Stop()
			inner := emit
			emit = func(ev figlens.StreamEvent) {
				stall.Touch()
				inner(ev)
			}
		}

		err = c.StreamChat(ctx, figlens.StreamParams{
			TaskID:    taskID,
			SessionID: flagWaitSessionID,
			Query:     "",
		}, emit)
		if err != nil {
			return clerr.Newf("stream interrupted: %s", err).WithCode(6)
		}
		if taskFailed {
			// Mirror `vk create`: user-fixable input → 2, retryable → 4,
			// otherwise 5. Keeps exit codes consistent across the two
			// stream-consuming entry points.
			code := 5
			switch {
			case httpclient.IsUserFixableCode(failedCode):
				code = 2
			case failedRetryable:
				code = 4
			}
			// wait is the reattach path, so it is often the only place a
			// terminal state is ever observed — a `--async` run's outcome
			// would otherwise never reach the ledger.
			noteJob(taskID, flagWaitSessionID, func(r *jobs.Record) {
				r.Status = jobs.StatusFailed
				r.Error = failedCode
			})
			return clerr.Newf("task failed").WithCode(code)
		}
		if taskPaused && !taskSucceeded {
			// Known non-terminal state — skip the probe: the backend told us
			// exactly where the run stands. Name the resume command rather
			// than the web editor, or a caller with no browser concludes the
			// run is lost and pays to create it again.
			noteJob(taskID, flagWaitSessionID, func(r *jobs.Record) {
				r.Status = jobs.StatusPaused
			})
			return clerr.Newf(
				"task paused; resume it with: vk video resume %d --session-id %s",
				taskID, flagWaitSessionID).WithCode(6)
		}
		if !taskSucceeded {
			// The stream closed without a terminal event. Exiting 0 here
			// would report success for a task whose state is unknown —
			// the worst failure mode for an agent caller, which cannot
			// tell "done" from "never ran". Exit 6 (task state unknown),
			// the same code a mid-run disconnect uses.
			//
			// Which of those it was is a question the backend can answer,
			// so ask it rather than handing the caller a guess: the reply
			// decides between reattaching and starting over, and getting
			// that wrong costs either the run or a second render.
			probe := cmdutil.ProbeRun(ctx, c, taskID, flagWaitSessionID)
			reattach := "vk video status --session-id " + flagWaitSessionID
			if probe.Delivery == cmdutil.DeliverySubmitted {
				reattach = "vk video wait --session-id " + flagWaitSessionID
			}
			if !sawAnyEvent {
				return cmdutil.UnknownStateError(
					"no events for this task: it has not started generating, or the --session-id does not match it",
					probe, reattach)
			}
			return cmdutil.UnknownStateError(
				"stream ended before the task reached a terminal state; its final state is unknown",
				probe, reattach)
		}

		w, err := c.GetWorkBySession(ctx, successSessionID)
		if err != nil {
			return err
		}
		s := snapshot.Build(snapshot.BuildInput{
			TaskID:    taskID,
			SessionID: successSessionID,
			Work:      w,
			ShareBase: cmdutil.ShareBaseURL(),
		})
		noteJob(taskID, successSessionID, func(r *jobs.Record) {
			r.Status = jobs.StatusSucceeded
			r.WorkID = s.WorkID
			r.Title = s.Title
			r.ShareURL = s.Preview.ShareURL
		})
		cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, c, w)
		return snapshot.Render(stdout, stderr, s, format)
	},
}

func init() {
	waitCmd.Flags().StringVar(&flagWaitSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	waitCmd.Flags().StringVar(&flagWaitPreviewDir, "preview-dir", "", i18n.T("create.flag.preview_dir"))
}
