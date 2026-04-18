package video

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/i18n"
)

var (
	flagListPage int
	flagListSize int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list your video works",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		works, total, err := c.ListWorks(context.Background(), flagListPage, flagListSize)
		if err != nil {
			return err
		}

		if len(works) == 0 {
			fmt.Println(i18n.T("video.list.empty"))
			return nil
		}

		// Table header
		fmt.Println(i18n.T("video.list.header",
			i18n.T("video.list.header.id"),
			i18n.T("video.list.header.title"),
			i18n.T("video.list.header.duration"),
			i18n.T("video.list.header.status"),
			i18n.T("video.list.header.created"),
		))
		fmt.Println(strings.Repeat("-", 80))

		for _, w := range works {
			title := w.Title
			if len(title) > 28 {
				title = title[:28] + ".."
			}

			duration := "--"
			if w.Duration > 0 {
				d := time.Duration(w.Duration) * time.Millisecond
				minutes := int(d.Minutes())
				seconds := int(d.Seconds()) % 60
				duration = fmt.Sprintf("%d:%02d", minutes, seconds)
			}

			status := i18n.T("video.list.status.unknown")
			switch w.Status {
			case 0:
				status = i18n.T("video.list.status.running")
			case 1:
				status = i18n.T("video.list.status.done")
			case 2:
				status = i18n.T("video.list.status.deleted")
			case 3:
				status = i18n.T("video.list.status.failed")
			}

			createdAt := w.CreatedAt
			if t, err := time.Parse(time.RFC3339, w.CreatedAt); err == nil {
				createdAt = t.Local().Format("2006-01-02 15:04")
			}

			fmt.Printf("%-8d  %-30s  %-8s  %-6s  %s\n", w.ID, title, duration, status, createdAt)
		}

		fmt.Println(i18n.T("video.list.footer", total, flagListPage))
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&flagListPage, "page", 1, "page number")
	listCmd.Flags().IntVar(&flagListSize, "size", 10, "page size")
}
