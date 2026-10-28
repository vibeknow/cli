package video

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var flagQuotaSessionID string

var quotaCmd = &cobra.Command{
	Use:   "quota [task_id]",
	Short: "show a work's free post-edit quota (what `vk video edit` costs nothing for)",
	Args:  cobra.MaximumNArgs(1),
	Long: `Show how much of a work's free post-edit quota is left.

Each work gets a rolling 24-hour quota (twice as much on a subscription),
in three parts:

  tts          seconds of regenerated speech — what ` + "`vk video edit`" + ` draws on
  layout       layout and image previews in the web editor
  background   background-image previews in the web editor

Within the tts quota an edit's speech is free; beyond it, it is billed when
the edit is saved. Text generation, and every image an edit draws, are
billed either way: ` + "`vk video edit`" + ` saves directly and does not go
through the web editor's preview tools, which are what the layout and
background quotas cover.

Read-only and free. The figures are a reading, not a reservation: another
edit — including one made on the web — can use them up before yours runs.`,
	Example: `  vk video quota
  vk video quota 42 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagQuotaSessionID)
		if err != nil {
			return err
		}
		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		q, err := c.GetEditQuota(cmd.Context(), sessionID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == output.FormatJSON || format == output.FormatNDJSON {
			payload := quotaMap(q)
			payload["task_id"] = taskID
			payload["session_id"] = sessionID
			return output.NewJSON(cmd.OutOrStdout()).Object(payload)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintln(out, i18n.T("quota.line.tts", q.TTS.Remaining, q.TTS.Total))
		fmt.Fprintln(out, i18n.T("quota.line.layout", q.Layout.Remaining, q.Layout.Total))
		fmt.Fprintln(out, i18n.T("quota.line.background", q.Background.Remaining, q.Background.Total))
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("quota.hint"))
		return nil
	},
}

// quotaMap is the JSON shape of a quota reading, shared by `video quota`
// and the edit confirmation. Units are in the key names so a caller does not
// have to know that TTS is counted in seconds and the others in results.
func quotaMap(q *figlens.EditQuota) map[string]any {
	return map[string]any{
		"subscribed": q.Subscribed,
		"tts":        usageMap(q.TTS, "_sec"),
		"layout":     usageMap(q.Layout, ""),
		"background": usageMap(q.Background, ""),
		// Which quota this CLI's edit actually draws on, stated with the
		// numbers so the reading cannot be mistaken for "edits are free":
		// layout and background cover the web editor's preview tools only.
		"video_edit_uses":              []string{"tts"},
		"billed_outside_quota_by_edit": []string{"text_generation", "image_generation"},
	}
}

func usageMap(u figlens.EditQuotaUsage, unit string) map[string]any {
	m := map[string]any{
		"used" + unit:      u.Used,
		"total" + unit:     u.Total,
		"remaining" + unit: u.Remaining,
		"exhausted":        u.Exhausted,
	}
	if u.ResetAt > 0 {
		m["resets_at"] = time.UnixMilli(u.ResetAt).UTC().Format(time.RFC3339)
	}
	return m
}

func init() {
	quotaCmd.Flags().StringVar(&flagQuotaSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
}
