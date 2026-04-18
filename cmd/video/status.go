package video

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
)

var flagStatusSessionID string

var statusCmd = &cobra.Command{
	Use:   "status <task_id>",
	Short: "get video task status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagStatusSessionID == "" {
			return clerr.Validation("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		w, err := c.GetWorkBySession(context.Background(), flagStatusSessionID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			payload := map[string]any{
				"task_id":    args[0],
				"session_id": flagStatusSessionID,
				"work_id":    w.ID,
				"title":      w.Title,
			}
			if w.VideoPath != "" {
				payload["video_path"] = w.VideoPath
			}
			if w.Duration > 0 {
				payload["duration"] = w.Duration
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(payload)
		}

		fmt.Printf("task_id=%s\n", args[0])
		fmt.Printf("session_id=%s\n", flagStatusSessionID)
		fmt.Printf("work_id=%d\n", w.ID)
		fmt.Printf("title=%s\n", w.Title)
		if w.VideoPath != "" {
			fmt.Printf("video_path=%s\n", w.VideoPath)
		}
		if w.Duration > 0 {
			fmt.Printf("duration=%d\n", w.Duration)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&flagStatusSessionID, "session-id", "", "session ID (required)")
}
