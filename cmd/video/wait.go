package video

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var flagWaitSessionID string

var waitCmd = &cobra.Command{
	Use:   "wait <task_id>",
	Short: "stream progress for a video task, block until done",
	Args:  cobra.ExactArgs(1),
	Example: `  vk video wait 123 --session-id sess_xxx
  vk video wait 123 --session-id sess_xxx --output ndjson`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagWaitSessionID == "" {
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

		ctx := cmd.Context()
		format, _ := cmd.Flags().GetString("output")
		stdout := cmd.OutOrStdout()
		stderr := cmd.ErrOrStderr()

		var emitEvent func(map[string]any) error
		if format == "ndjson" {
			w := output.NewNDJSON(stdout)
			emitEvent = w.Event
		}

		var taskFailed, taskSucceeded bool
		var successSessionID string

		emit := func(ev figlens.StreamEvent) {
			if emitEvent != nil {
				out := map[string]any{
					"type":    ev.Type,
					"stage":   ev.Stage,
					"node":    ev.Node,
					"message": ev.Message,
				}
				if ev.SessionID != "" {
					out["session_id"] = ev.SessionID
				}
				_ = emitEvent(out)
			} else {
				switch ev.Type {
				case "node.started":
					fmt.Fprintf(stderr, "[%s] started\n", ev.Node)
				case "node.succeeded":
					fmt.Fprintf(stderr, "[%s] done\n", ev.Node)
				case "node.failed":
					fmt.Fprintf(stderr, "[%s] failed: %s\n", ev.Node, ev.Message)
				case "task.succeeded":
					fmt.Fprintf(stderr, "task succeeded\n")
				case "task.failed":
					fmt.Fprintf(stderr, "task failed: %s\n", ev.Message)
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
			return clerr.Newf("task failed").WithCode(5)
		}
		if !taskSucceeded {
			return nil
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
		return snapshot.Render(stdout, stderr, s, format)
	},
}

func init() {
	waitCmd.Flags().StringVar(&flagWaitSessionID, "session-id", "", "session ID (required)")
}
