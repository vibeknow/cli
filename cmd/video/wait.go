package video

import (
	"context"
	"fmt"
	"os"
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

		taskIDStr := args[0]
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			return clerr.Validationf("task_id must be an integer: %v", err)
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := context.Background()
		format, _ := cmd.Flags().GetString("output")
		isNDJSON := format == "ndjson"

		var taskFailed, taskSucceeded bool
		var successSessionID string

		emit := func(ev figlens.StreamEvent) {
			if isNDJSON {
				out := map[string]any{
					"type":    ev.Type,
					"stage":   ev.Stage,
					"node":    ev.Node,
					"message": ev.Message,
				}
				if ev.SessionID != "" {
					out["session_id"] = ev.SessionID
				}
				_ = output.NewNDJSON(cmd.OutOrStdout()).Event(out)
			} else {
				switch ev.Type {
				case "node.started":
					fmt.Fprintf(os.Stderr, "[%s] started\n", ev.Node)
				case "node.succeeded":
					fmt.Fprintf(os.Stderr, "[%s] done\n", ev.Node)
				case "node.failed":
					fmt.Fprintf(os.Stderr, "[%s] failed: %s\n", ev.Node, ev.Message)
				case "task.succeeded":
					fmt.Fprintf(os.Stderr, "task succeeded\n")
				case "task.failed":
					fmt.Fprintf(os.Stderr, "task failed: %s\n", ev.Message)
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
			fmt.Fprintf(os.Stderr, "stream interrupted: %s\n", err)
			os.Exit(6)
		}
		if taskFailed {
			os.Exit(5)
		}
		if !taskSucceeded {
			return nil
		}

		// Fetch work + render final payload.
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

		if isNDJSON {
			return snapshot.RenderNDJSON(cmd.OutOrStdout(), s)
		}
		if format == "json" {
			return snapshot.RenderJSON(cmd.OutOrStdout(), s)
		}
		snapshot.RenderText(cmd.OutOrStdout(), cmd.ErrOrStderr(), s)
		return nil
	},
}

func init() {
	waitCmd.Flags().StringVar(&flagWaitSessionID, "session-id", "", "session ID (required)")
}
