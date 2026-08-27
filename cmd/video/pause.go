package video

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var (
	flagPauseSessionID  string
	flagResumeSessionID string
)

var pauseCmd = &cobra.Command{
	Use:   "pause [task_id]",
	Short: "stop a run that is currently generating",
	Args:  cobra.MaximumNArgs(1),
	Long: `Stop a generating run. The work it has done so far is kept, and
` + "`vk video resume`" + ` continues from that point rather than starting over.

Use this when a run was started by mistake, or on the wrong document: the
alternative is letting it finish and paying for a video nobody wanted.

Only a run that is still generating can be paused — one that has already
finished, failed, or been paused cannot.`,
	Example: `  vk video pause
  vk video pause 42
  vk video pause 42 --session-id s_abc`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagPauseSessionID)
		if err != nil {
			return err
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		if err := c.PausePipeline(cmd.Context(), sessionID); err != nil {
			return classifyPipelineControlError(err)
		}

		noteJob(taskID, sessionID, func(r *jobs.Record) { r.Status = jobs.StatusPaused })

		format, _ := cmd.Flags().GetString("output")
		if format == "json" || format == "ndjson" {
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"session_id": sessionID,
				"task_id":    taskID,
				"status":     "paused",
				"next_actions": []map[string]string{{
					"command": fmt.Sprintf("vk video resume %s", snapshot.Target(taskID, sessionID)),
					"purpose": "Continue this run from where it stopped",
				}},
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "paused — resume with `vk video resume %s`\n", snapshot.Target(taskID, sessionID))
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume [task_id]",
	Short: "continue a paused run, or retry a failed one from its checkpoint",
	Args:  cobra.MaximumNArgs(1),
	Long: `Continue a run that was paused, or restart a failed one from its last
checkpoint instead of from the beginning.

The failed case is the one worth knowing about: the backend reopens the
original bill, so retrying costs far less than the obvious alternative of
creating the video again — and it keeps every scene that already succeeded.

Three refusals are permanent, and no retry can clear them:
  - the run used --engine agent, which keeps no checkpoint
  - it was stopped by the provider's content policy, which the same inputs
    would trigger again
  - it is neither paused nor failed (a finished run has nothing to resume)`,
	Example: `  vk video resume
  vk video resume 42
  vk video resume 42 --session-id s_abc`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagResumeSessionID)
		if err != nil {
			return err
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		mode, err := c.ResumePipeline(cmd.Context(), sessionID)
		if err != nil {
			return classifyPipelineControlError(err)
		}

		noteJob(taskID, sessionID, func(r *jobs.Record) { r.Status = jobs.StatusRunning })

		waitCmd := fmt.Sprintf("vk video wait %s", snapshot.Target(taskID, sessionID))
		format, _ := cmd.Flags().GetString("output")
		if format == "json" || format == "ndjson" {
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"session_id": sessionID,
				"task_id":    taskID,
				"status":     "resumed",
				// Which of the two things happened, because they mean
				// different things to the caller: one continued work the user
				// stopped, the other retried work that broke.
				"mode": string(mode),
				"next_actions": []map[string]string{{
					"command": waitCmd,
					"purpose": "Follow the run to its terminal state",
				}},
			})
		}
		switch mode {
		case figlens.ResumeModeFailedRetry:
			fmt.Fprintf(cmd.OutOrStdout(), "retrying from the last checkpoint — follow it with `%s`\n", waitCmd)
		default:
			fmt.Fprintf(cmd.OutOrStdout(), "resumed — follow it with `%s`\n", waitCmd)
		}
		return nil
	},
}

// classifyPipelineControlError gives pause/resume failures an exit code an
// agent can act on.
//
// The backend answers every one of these with HTTP 400 and a sentence, so
// they all arrive indistinguishable and land on exit 2 — the code that means
// "your input is wrong, correct it and retry". For this pair that is wrong
// nearly every time, and wrong in the most expensive direction: "the work is
// not in a state that can be paused", "this engine keeps no checkpoint" and
// "content policy stopped it" are settled facts about the *run*, not about
// the arguments. An agent told to fix its input will keep permuting flags
// against a condition no flag can reach.
//
// So the default here is 5 (the run cannot proceed, tell the user why) and
// the exception is carved out rather than the other way round. Only a
// concurrent pause/resume on the same session is genuinely worth retrying,
// and the backend names it. Matching on that sentence is fragile, which is
// why it is the *narrow* side: if the wording drifts, a retryable case gets
// reported as permanent — the user is told, rather than the CLI hammering an
// endpoint on their behalf.
func classifyPipelineControlError(err error) error {
	var o *errs.Object
	if !errors.As(err, &o) {
		return err
	}
	if strings.Contains(strings.ToLower(o.Message), "session busy") {
		return clerr.Newf("%s", o.Message).WithCode(4).WithCause(err)
	}
	return clerr.Newf("%s", o.Message).WithCode(5).WithCause(err)
}

func init() {
	pauseCmd.Flags().StringVar(&flagPauseSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	resumeCmd.Flags().StringVar(&flagResumeSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
}
