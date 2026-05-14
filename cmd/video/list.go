package video

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
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

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			shareBase := cmdutil.ShareBaseURL()
			items := make([]map[string]any, 0, len(works))
			for _, w := range works {
				item := map[string]any{
					"id":          w.ID,
					"session_id":  w.SessionID,
					"title":       w.Title,
					"duration_ms": w.Duration,
					"status":      w.Status,
					"exporting":   w.Exporting,
					"created_at":  w.CreatedAt,
				}
				if w.ShareToken != "" {
					item["share_url"] = snapshot.ShareURL(shareBase, w.ShareToken)
				}
				if w.VideoPath != "" {
					item["video_path"] = w.VideoPath
				}
				if w.Engine != "" {
					item["engine"] = figlens.RemapEngineForDisplay(w.Engine)
				}
				items = append(items, item)
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"works": items,
				"total": total,
				"page":  flagListPage,
			})
		}

		if len(works) == 0 {
			fmt.Println(i18n.T("video.list.empty"))
			return nil
		}
		fmt.Println(i18n.T("video.list.header.ext",
			i18n.T("video.list.header.id"),
			i18n.T("video.list.header.title"),
			i18n.T("video.list.header.duration"),
			i18n.T("video.list.header.status"),
			i18n.T("video.list.header.share"),
			i18n.T("video.list.header.created"),
		))
		fmt.Println(strings.Repeat("-", 100))
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
			status := mapStatus(w.Status, w.Exporting == 1)
			share := "--"
			if w.ShareToken != "" {
				share = "yes"
			}
			createdAt := w.CreatedAt
			if t, err := time.Parse(time.RFC3339, w.CreatedAt); err == nil {
				createdAt = t.Local().Format("2006-01-02 15:04")
			}
			fmt.Printf("%-8d  %-30s  %-8s  %-10s  %-6s  %s\n", w.ID, title, duration, status, share, createdAt)
		}
		fmt.Println(i18n.T("video.list.footer", total, flagListPage))
		return nil
	},
}

func mapStatus(s int, exporting bool) string {
	if exporting {
		return i18n.T("video.list.status.exporting")
	}
	switch s {
	case 0:
		return i18n.T("video.list.status.running")
	case 1:
		return i18n.T("video.list.status.done")
	case 2:
		return i18n.T("video.list.status.deleted")
	case 3:
		return i18n.T("video.list.status.failed")
	}
	return i18n.T("video.list.status.unknown")
}

func init() {
	listCmd.Flags().IntVar(&flagListPage, "page", 1, "page number")
	listCmd.Flags().IntVar(&flagListSize, "size", 10, "page size")
}
