package video

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagCoverSessionID  string
	flagCoverTitle      string
	flagCoverSubtitle   string
	flagCoverYes        bool
	flagCoverConfirm    string
	flagCoverPreviewDir string
)

var coverCmd = &cobra.Command{
	Use:   "cover [task_id]",
	Short: "generate an AI cover for a video and apply it (bills)",
	Args:  cobra.MaximumNArgs(1),
	Long: `Generate a new AI cover image for a finished video and make it the
video's cover. The previous covers stay on the work as candidates.

--title is the text drawn on the cover; it defaults to the video's title.
--subtitle adds a second line.

**This bills** one image generation, charged only once the cover has been
saved: a generation that fails costs nothing. The backend does not publish
the unit price, so the confirmation names the billed item rather than a
figure. Without a terminal the decision is handed back as exit 8; see
` + "`vk video export`" + ` for the same gate.

**An exported MP4 is withdrawn.** The rendered video shows the old cover, so
the backend clears it; ` + "`vk video download`" + ` has nothing to return
until the next ` + "`vk video export`" + `. The result says so as
export_cleared.

Generation takes 30–60 seconds. If the connection drops before the result
arrives, the outcome is unknown — the backend finishes and bills a
generation whether or not anyone is listening — so the command exits 6
rather than suggesting a retry. Check ` + "`vk video status`" + ` for the
cover_url before generating again.`,
	Example: `  vk video cover 42
  vk video cover 42 --title "三分钟看懂季度财报" --subtitle "2026 Q3"
  vk video cover 42 --confirm act_9f2c... --preview-dir ./out`,
	RunE: runCover,
}

func runCover(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	taskID, sessionID, err := resolveTarget(ctx, args, flagCoverSessionID)
	if err != nil {
		return err
	}
	c, err := newFiglensClient()
	if err != nil {
		return err
	}
	ch, err := cmdutil.NewRunChannel(cmd, flagCoverPreviewDir)
	if err != nil {
		return clerr.Validation(err.Error())
	}
	work, err := c.GetWorkBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	if taskID == 0 {
		taskID = work.TaskID
	}
	switch work.Status {
	case figlens.WorkStatusActive:
	case figlens.WorkStatusFailed, figlens.WorkStatusDeleted:
		// Terminal: waiting will not make a preview appear.
		return clerr.New(i18n.T("cover.err.no_preview")).WithCode(5)
	default:
		return clerr.New(i18n.T("cover.err.not_ready")).WithCode(6).
			WithHint("check `vk video status`; a cover can be generated once the preview is ready")
	}

	title := strings.TrimSpace(flagCoverTitle)
	if title == "" {
		title = strings.TrimSpace(work.Title)
	}
	if title == "" {
		return clerr.Validation(i18n.T("cover.err.no_title"))
	}
	subtitle := strings.TrimSpace(flagCoverSubtitle)
	hadExport := strings.TrimSpace(work.VideoPath) != ""

	prompt := i18n.T("cover.confirm_prompt", title)
	if hadExport {
		prompt += " " + i18n.T("cover.confirm_export")
	}
	prompt += " " + i18n.T("cover.confirm_tail")
	ok, err := cmdutil.Gate(cmd, cmdutil.GateOptions{
		Type:    cmdutil.CoverActionType,
		Payload: cmdutil.CoverActionPayload(sessionID, title, subtitle),
		Prompt:  prompt,
		Yes:     flagCoverYes,
		Token:   flagCoverConfirm,
		ResumeCommand: func(token string) string {
			cmdline := fmt.Sprintf("vk video cover %s --title %s", targetArgs(taskID, sessionID), shellQuote(title))
			if subtitle != "" {
				cmdline += " --subtitle " + shellQuote(subtitle)
			}
			return cmdline + " --confirm " + token
		},
	})
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("cover.cancelled"))
		return nil
	}

	if !ch.Structured() {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("cover.generating"))
	}
	res, err := c.GenerateCover(ctx, figlens.GenerateCoverParams{WorkID: work.ID, Title: title, Subtitle: subtitle})
	if errors.Is(err, figlens.ErrCoverOutcomeUnknown) {
		// The one answer that must not be "retry": the backend may well
		// have made and billed the cover after the connection went.
		return clerr.New(i18n.T("cover.err.outcome_unknown")).WithCode(6).
			WithDetail(map[string]any{"resend_safe": false}).
			WithHintf("run `vk video status %s` and compare cover_url before generating again", targetArgs(taskID, sessionID))
	}
	if err != nil {
		return err
	}

	exportCleared := hadExport && strings.TrimSpace(res.VideoPath) == ""
	if exportCleared {
		noteJob(taskID, sessionID, func(r *jobs.Record) { r.VideoPath = "" })
	}
	if ch.Previews != nil {
		if w, err := c.GetWorkBySession(ctx, sessionID); err == nil {
			cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, c, w)
		}
	}

	format, _ := cmd.Flags().GetString("output")
	if format == output.FormatJSON || format == output.FormatNDJSON {
		payload := map[string]any{
			"task_id":        taskID,
			"session_id":     sessionID,
			"work_id":        work.ID,
			"cover_url":      res.CoverURL,
			"cover_urls":     res.CoverURLs,
			"title":          title,
			"export_cleared": exportCleared,
		}
		if subtitle != "" {
			payload["subtitle"] = subtitle
		}
		if exportCleared {
			payload["next_actions"] = []map[string]string{{
				"command": "vk video export " + targetArgs(taskID, sessionID),
				"purpose": "Re-render the MP4 with the new cover (bills; asks for confirmation first)",
			}}
		}
		return output.NewJSON(cmd.OutOrStdout()).Object(payload)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cover_url=%s\n", res.CoverURL)
	if exportCleared {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("cover.export_cleared", targetArgs(taskID, sessionID)))
	}
	return nil
}

func init() {
	coverCmd.Flags().StringVar(&flagCoverSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	coverCmd.Flags().StringVar(&flagCoverTitle, "title", "", "text drawn on the cover (default: the video's title)")
	coverCmd.Flags().StringVar(&flagCoverSubtitle, "subtitle", "", "optional second line on the cover")
	coverCmd.Flags().BoolVarP(&flagCoverYes, "yes", "y", false, "skip the confirmation: only when the user has already authorised this spend")
	coverCmd.Flags().StringVar(&flagCoverConfirm, "confirm", "", "action_id from a previously blocked run, once the user has agreed")
	coverCmd.Flags().StringVar(&flagCoverPreviewDir, "preview-dir", "", "write the new cover here and announce it as a resource_ready event")
}
