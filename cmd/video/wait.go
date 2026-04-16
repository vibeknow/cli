package video

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
)

var flagWaitSessionID string

var waitCmd = &cobra.Command{
	Use:   "wait <task_id>",
	Short: "stream progress for a video task, block until done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagWaitSessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		taskIDStr := args[0]
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			return fmt.Errorf("task_id must be an integer: %w", err)
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := context.Background()
		var taskFailed bool
		var taskSucceeded bool
		var successSessionID string

		err = c.StreamChat(ctx, figlens.StreamParams{
			TaskID:    taskID,
			SessionID: flagWaitSessionID,
			Query:     "",
		}, func(ev figlens.StreamEvent) {
			switch ev.Type {
			case "node.started":
				fmt.Fprintf(os.Stderr, "[%s] started\n", ev.Node)
			case "node.succeeded":
				fmt.Fprintf(os.Stderr, "[%s] done\n", ev.Node)
			case "node.failed":
				fmt.Fprintf(os.Stderr, "[%s] failed: %s\n", ev.Node, ev.Message)
			case "task.succeeded":
				taskSucceeded = true
				successSessionID = ev.SessionID
				if successSessionID == "" {
					successSessionID = flagWaitSessionID
				}
				fmt.Fprintf(os.Stderr, "task succeeded\n")
			case "task.failed":
				taskFailed = true
				fmt.Fprintf(os.Stderr, "task failed: %s\n", ev.Message)
			}
		})
		if err != nil {
			// stream interrupted
			fmt.Fprintf(os.Stderr, "stream interrupted: %s\n", err)
			os.Exit(6)
		}

		if taskFailed {
			os.Exit(5)
		}

		if taskSucceeded {
			w, err := c.GetWorkBySession(ctx, successSessionID)
			if err != nil {
				return err
			}
			fmt.Printf("work_id=%d\n", w.ID)
			fmt.Printf("title=%s\n", w.Title)
			if w.VideoPath != "" {
				fmt.Printf("video_path=%s\n", w.VideoPath)
			}
			if w.Duration > 0 {
				fmt.Printf("duration=%d\n", w.Duration)
			}
		}

		return nil
	},
}

func init() {
	waitCmd.Flags().StringVar(&flagWaitSessionID, "session-id", "", "session ID (required)")
}
