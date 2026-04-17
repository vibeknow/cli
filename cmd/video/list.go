package video

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
			fmt.Println("暂无作品")
			return nil
		}

		// Table header
		fmt.Printf("%-8s  %-30s  %-8s  %-6s  %s\n", "ID", "标题", "时长", "状态", "创建时间")
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

			status := "未知"
			switch w.Status {
			case 0:
				status = "生成中"
			case 1:
				status = "完成"
			case 2:
				status = "已删除"
			case 3:
				status = "失败"
			}

			createdAt := w.CreatedAt
			if t, err := time.Parse(time.RFC3339, w.CreatedAt); err == nil {
				createdAt = t.Local().Format("2006-01-02 15:04")
			}

			fmt.Printf("%-8d  %-30s  %-8s  %-6s  %s\n", w.ID, title, duration, status, createdAt)
		}

		fmt.Printf("\n共 %d 条，第 %d 页\n", total, flagListPage)
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&flagListPage, "page", 1, "page number")
	listCmd.Flags().IntVar(&flagListSize, "size", 10, "page size")
}
