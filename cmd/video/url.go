package video

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
)

var urlCmd = &cobra.Command{
	Use:   "url <work_id>",
	Short: "get a playable video URL for a work",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return clerr.Validationf("invalid work_id: %s", args[0])
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		url, err := c.GetVideoURL(context.Background(), workID)
		if err != nil {
			return err
		}

		fmt.Println(url)
		return nil
	},
}
