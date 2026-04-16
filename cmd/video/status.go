package video

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var flagStatusSessionID string

var statusCmd = &cobra.Command{
	Use:   "status <task_id>",
	Short: "get video task status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagStatusSessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		w, err := c.GetWorkBySession(context.Background(), flagStatusSessionID)
		if err != nil {
			return err
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
