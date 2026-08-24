package jobs

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/durfmt"
	"github.com/vibeknow/cli/internal/jobs"
)

var (
	flagListLimit  int
	flagListActive bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list recorded runs, newest first",
	Example: `  vk jobs list
  vk jobs list --active --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		all, err := jobs.Load()
		if err != nil {
			return err
		}
		if flagListActive {
			var open []jobs.Record
			for _, r := range all {
				if !r.Terminal() {
					open = append(open, r)
				}
			}
			all = open
		}
		if flagListLimit > 0 && len(all) > flagListLimit {
			all = all[:flagListLimit]
		}

		items := make([]map[string]any, 0, len(all))
		for _, r := range all {
			items = append(items, recordMap(r))
		}
		return cmdutil.Emit(cmd, map[string]any{
			"jobs":  items,
			"total": len(items),
		}, "jobs.list", func(w io.Writer) {
			if len(all) == 0 {
				fmt.Fprintln(w, "no recorded runs (the ledger fills up as `vk create` runs)")
				return
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TASK_ID\tSESSION_ID\tSTATUS\tAGE\tSOURCE")
			for _, r := range all {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n",
					r.TaskID, r.SessionID, r.Status, age(r.UpdatedAt), truncate(r.Source, 40))
			}
			_ = tw.Flush()
		})
	},
}

// recordMap is the JSON shape of a ledger entry. It is shared with `jobs
// get` so both commands describe a run identically.
func recordMap(r jobs.Record) map[string]any {
	m := map[string]any{
		"task_id":    r.TaskID,
		"session_id": r.SessionID,
		"status":     r.Status,
		"terminal":   r.Terminal(),
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
	for k, v := range map[string]string{
		"source":     r.Source,
		"mode":       r.Mode,
		"engine":     r.Engine,
		"title":      r.Title,
		"share_url":  r.ShareURL,
		"video_path": r.VideoPath,
		"error":      r.Error,
	} {
		if v != "" {
			m[k] = v
		}
	}
	if r.WorkID != 0 {
		m["work_id"] = r.WorkID
	}
	return m
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return durfmt.Short(time.Since(t))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	listCmd.Flags().IntVar(&flagListLimit, "limit", 20, "maximum rows to show (0 for all)")
	listCmd.Flags().BoolVar(&flagListActive, "active", false, "only runs that have not reached a terminal state")
}
