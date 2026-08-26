package video

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagSetSessionID     string
	flagSetTitle         string
	flagSetBGM           string
	flagSetBGMVolume     float64
	flagSetSubtitle      string
	flagSetSubtitleSize  int
	flagSetSubtitleColor string
	flagSetSubtitleFont  string
)

var setCmd = &cobra.Command{
	Use:   "set [task_id]",
	Short: "change a finished work's music, subtitles or title",
	Args:  cobra.MaximumNArgs(1),
	Long: `Adjust a video that has already been generated, without regenerating it.

These are the changes that do not need the pipeline to run again — the music,
the subtitles, the title — so they cost nothing and take effect immediately.
Anything about the *content* (the narration, the images, the layout) still
requires a fresh ` + "`create`" + `.

Only the settings you name are touched. Subtitle style in particular is
merged onto whatever the work already has: the backend stores the style
wholesale, so changing the size by sending only the size would clear the
font, colour and outline along with it.

**The rendered MP4 is discarded by any of these changes.** It has to be, since
the change is baked into the file. The preview and share link keep working;
the download does not, until the next ` + "`vk video export`" + `, which bills.
Renaming is the exception — it touches no output.`,
	Example: `  vk video set --bgm off
  vk video set 42 --bgm-volume 0.6
  vk video set 42 --subtitle on --subtitle-size 48
  vk video set 42 --title "季度复盘"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		changed := func(name string) bool { return cmd.Flags().Changed(name) }

		touchesWork := changed("bgm") || changed("bgm-volume") || changed("subtitle") ||
			changed("subtitle-size") || changed("subtitle-color") || changed("subtitle-font")
		if !touchesWork && !changed("title") {
			return clerr.Validation("nothing to change").
				WithHint("pass at least one of --bgm, --bgm-volume, --subtitle, --subtitle-size, --subtitle-color, --subtitle-font, --title")
		}

		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagSetSessionID)
		if err != nil {
			return err
		}
		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		applied := map[string]any{}

		// Renaming is addressed by task and changes no output, so it is done
		// first and independently of everything below.
		if changed("title") {
			if strings.TrimSpace(flagSetTitle) == "" {
				return clerr.Validation("--title cannot be empty")
			}
			if taskID == 0 {
				return clerr.Validation("--title needs a task_id").
					WithHint("pass it as the argument, or run this right after a create so the ledger has it")
			}
			if err := c.RenameTask(cmd.Context(), taskID, flagSetTitle); err != nil {
				return err
			}
			applied["title"] = flagSetTitle
		}

		if touchesWork {
			work, err := c.GetWorkBySession(cmd.Context(), sessionID)
			if err != nil {
				return err
			}

			if changed("bgm") {
				state, err := onOff("--bgm", flagSetBGM)
				if err != nil {
					return err
				}
				if err := c.SetBGM(cmd.Context(), work.ID, state); err != nil {
					return err
				}
				applied["bgm"] = state
			}
			if changed("bgm-volume") {
				if err := c.SetBGMVolume(cmd.Context(), work.ID, flagSetBGMVolume); err != nil {
					// The range check happens client-side, so a rejection
					// here is the engine refusing the feature — permanent for
					// this work, not something a different number fixes.
					return classifySettingError(err)
				}
				applied["bgm_volume"] = flagSetBGMVolume
			}
			if changed("subtitle") {
				state, err := onOff("--subtitle", flagSetSubtitle)
				if err != nil {
					return err
				}
				if err := c.SetSubtitle(cmd.Context(), work.ID, state); err != nil {
					return err
				}
				applied["subtitle"] = state
			}

			// Style is read-modify-write, never write-only.
			if changed("subtitle-size") || changed("subtitle-color") || changed("subtitle-font") {
				style := figlens.ParseSubtitleStyle(work.SubtitleStyle)
				if changed("subtitle-size") {
					if flagSetSubtitleSize <= 0 {
						return clerr.Validation("--subtitle-size must be positive")
					}
					style.FontSize = flagSetSubtitleSize
				}
				if changed("subtitle-color") {
					style.Color = flagSetSubtitleColor
				}
				if changed("subtitle-font") {
					style.FontFamily = flagSetSubtitleFont
				}
				if err := c.SetSubtitleStyle(cmd.Context(), work.ID, style); err != nil {
					return classifySettingError(err)
				}
				applied["subtitle_style"] = style
			}
		}

		// Saying the download is gone is not optional. A user who turned the
		// music off and later finds no file would have no way to connect the
		// two, and an agent that does not know cannot warn them.
		exportInvalidated := touchesWork

		format, _ := cmd.Flags().GetString("output")
		if format == "json" || format == "ndjson" {
			payload := map[string]any{
				"task_id":            taskID,
				"session_id":         sessionID,
				"applied":            applied,
				"export_invalidated": exportInvalidated,
			}
			if exportInvalidated {
				payload["next_actions"] = []map[string]string{{
					"command": fmt.Sprintf("vk video export %d --session-id %s", taskID, sessionID),
					"purpose": "Re-render the MP4 with the change baked in (bills; asks for confirmation first)",
				}}
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(payload)
		}

		out := cmd.OutOrStdout()
		for k, v := range applied {
			fmt.Fprintf(out, "%s = %v\n", k, v)
		}
		if exportInvalidated {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"note: the previously rendered MP4 no longer matches this work and was discarded; "+
					"the preview and share link still work, run `vk video export %d --session-id %s` to get a file again\n",
				taskID, sessionID)
		}
		return nil
	},
}

// onOff validates the two-valued flags, naming the flag so the message points
// at what to fix rather than at the wire value.
func onOff(flag, v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on":
		return "on", nil
	case "off":
		return "off", nil
	}
	return "", clerr.Validationf("%s must be on or off, got %q", flag, v)
}

// classifySettingError separates "this work cannot have that" from a passing
// failure.
//
// The backend answers both with HTTP 400, which lands on exit 2 — fix your
// input and retry. For an engine that does not support BGM volume, or a work
// whose renderer cannot carry styled subtitles, no input fixes it; exit 5 says
// the request was fine and the thing still cannot be done, so the answer is to
// tell the user rather than keep trying values.
func classifySettingError(err error) error {
	msg := strings.ToLower(err.Error())
	for _, permanent := range []string{"not supported", "not capable", "unsupported"} {
		if strings.Contains(msg, permanent) {
			return clerr.Newf("%s", err.Error()).WithCode(5).WithCause(err)
		}
	}
	return err
}

func init() {
	setCmd.Flags().StringVar(&flagSetSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	setCmd.Flags().StringVar(&flagSetTitle, "title", "", "rename the task (changes no rendered output)")
	setCmd.Flags().StringVar(&flagSetBGM, "bgm", "", "background music: on or off")
	setCmd.Flags().Float64Var(&flagSetBGMVolume, "bgm-volume", 0, "background music level, 0.1–2.0 (1.0 = unchanged)")
	setCmd.Flags().StringVar(&flagSetSubtitle, "subtitle", "", "subtitles: on or off")
	setCmd.Flags().IntVar(&flagSetSubtitleSize, "subtitle-size", 0, "subtitle font size in px")
	setCmd.Flags().StringVar(&flagSetSubtitleColor, "subtitle-color", "", "subtitle text colour, e.g. #FFFFFF")
	setCmd.Flags().StringVar(&flagSetSubtitleFont, "subtitle-font", "", "subtitle font family (must be one the backend allows)")
}
