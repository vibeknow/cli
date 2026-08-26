package video

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagSetSessionID           string
	flagSetTitle               string
	flagSetBGM                 string
	flagSetBGMVolume           float64
	flagSetSubtitle            string
	flagSetSubtitlePreset      string
	flagSetSubtitleSize        int
	flagSetSubtitleColor       string
	flagSetSubtitleFont        string
	flagSetSubtitleFontWeight  int
	flagSetSubtitleBgColor     string
	flagSetSubtitleBottom      float64
	flagSetSubtitleStrokeColor string
	flagSetSubtitleStrokeWidth float64
	flagSetSubtitleAnimation   string
)

// subtitleStyleFlags are the flags that write the work's subtitle style.
//
// The list lives in one place because three things need exactly it: deciding
// whether the work is touched at all, deciding whether to read-modify-write
// the style, and telling a caller who passed nothing what they could have
// passed. Kept apart, the third drifts first and quietly stops mentioning
// flags that exist.
var subtitleStyleFlags = []string{
	"subtitle-preset", "subtitle-size", "subtitle-color", "subtitle-font",
	"subtitle-font-weight", "subtitle-bg-color", "subtitle-bottom",
	"subtitle-stroke-color", "subtitle-stroke-width", "subtitle-animation",
}

// workFlags are every flag that changes the work itself, and therefore every
// flag that discards the rendered MP4. --title is not among them.
var workFlags = append([]string{"bgm", "bgm-volume", "subtitle"}, subtitleStyleFlags...)

var setCmd = &cobra.Command{
	Use:   "set [task_id]",
	Short: "change a finished work's music, subtitles or title",
	Args:  cobra.MaximumNArgs(1),
	Long: `Adjust a video that has already been generated, without regenerating it.

These are the changes that do not need the pipeline to run again — the music,
the subtitles, the title — so they cost nothing and take effect immediately.
Anything about the *content* (the narration, the images, the layout) is a
different matter: narration is changed with ` + "`vk video edit`" + `, which bills;
images and layout still have no command.

Only the settings you name are touched. Subtitle style in particular is
merged onto whatever the work already has: the backend stores the style
wholesale, so changing the size by sending only the size would clear the
font, colour and outline along with it.

For subtitle appearance, reach for ` + "`--subtitle-preset`" + ` before the individual
colour and outline flags. Readability is a combination rather than a set of
independent settings — the outlined looks also clear the background plate, and
the plated looks also switch the outline off — and a preset gets the whole
combination right. Run ` + "`vk subtitle presets`" + ` to see them and
` + "`vk subtitle fonts`" + ` for the families that are allowed. Individual flags
still apply on top of a preset, so you can take a look and change one thing.

**The rendered MP4 is discarded by any of these changes.** It has to be, since
the change is baked into the file. The preview and share link keep working;
the download does not, until the next ` + "`vk video export`" + `, which bills.
Renaming is the exception — it touches no output.`,
	Example: `  vk video set --bgm off
  vk video set 42 --bgm-volume 0.6
  vk video set 42 --subtitle on --subtitle-preset 2
  vk video set 42 --subtitle-preset "白字·黑边" --subtitle-size 48
  vk video set 42 --title "季度复盘"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		changed := func(name string) bool { return cmd.Flags().Changed(name) }
		anyChanged := func(names []string) bool {
			for _, n := range names {
				if changed(n) {
					return true
				}
			}
			return false
		}

		touchesWork := anyChanged(workFlags)
		touchesStyle := anyChanged(subtitleStyleFlags)
		if !touchesWork && !changed("title") {
			return clerr.Validation("nothing to change").
				WithHintf("pass at least one of --%s, --title", strings.Join(workFlags, ", --"))
		}

		// Everything checkable without the network is checked before the
		// network. A bad number should cost nothing, and should not leave
		// half the settings applied because it was caught after the first
		// write went out.
		if err := validateSubtitleStyleFlags(cmd); err != nil {
			return err
		}

		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagSetSessionID)
		if err != nil {
			return err
		}
		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		// The two catalog-backed flags resolve next, still before any write.
		// Both accept a display index or the real value, and both refuse
		// locally with the list of what is valid rather than letting the
		// backend answer "not allowed" with no way to find out what is.
		var preset figlens.SubtitlePreset
		if changed("subtitle-preset") {
			preset, err = resolveSubtitlePreset(cmd.Context(), c, flagSetSubtitlePreset)
			if err != nil {
				return err
			}
		}
		fontFamily := flagSetSubtitleFont
		if changed("subtitle-font") {
			fontFamily, err = resolveSubtitleFont(cmd.Context(), c, flagSetSubtitleFont)
			if err != nil {
				return err
			}
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
			if touchesStyle {
				style := figlens.ParseSubtitleStyle(work.SubtitleStyle)

				// The preset goes on first so that a flag passed alongside it
				// wins. That order is the useful one: "this look, but bigger"
				// is a thing people ask for; "bigger, but whatever the look
				// says" is not.
				if changed("subtitle-preset") {
					style = preset.Patch.Apply(style)
					applied["subtitle_preset"] = preset.Name
				}
				if changed("subtitle-size") {
					style.FontSize = flagSetSubtitleSize
				}
				if changed("subtitle-color") {
					style.Color = flagSetSubtitleColor
				}
				if changed("subtitle-font") {
					style.FontFamily = fontFamily
				}
				if changed("subtitle-font-weight") {
					style.FontWeight = flagSetSubtitleFontWeight
				}
				if changed("subtitle-bg-color") {
					style.BackgroundColor = flagSetSubtitleBgColor
				}
				if changed("subtitle-bottom") {
					style.BottomPercent = flagSetSubtitleBottom
				}
				if changed("subtitle-stroke-color") {
					style.StrokeColor = flagSetSubtitleStrokeColor
				}
				if changed("subtitle-stroke-width") {
					style.StrokeWidth = flagSetSubtitleStrokeWidth
				}
				if changed("subtitle-animation") {
					// Already validated; normalizing again is what actually
					// puts the lower-cased value on the wire.
					anim, _ := figlens.NormalizeSubtitleAnimation(flagSetSubtitleAnimation)
					style.Animation = anim
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
		// Sorted, because `applied` is a map and an unordered report of what
		// changed reads as a different result every run.
		keys := make([]string, 0, len(applied))
		for k := range applied {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if style, ok := applied[k].(figlens.SubtitleStyle); ok {
				fmt.Fprintf(out, "%s:\n", k)
				for _, line := range describeSubtitleStyle(style) {
					fmt.Fprintf(out, "  %s\n", line)
				}
				continue
			}
			fmt.Fprintf(out, "%s = %v\n", k, applied[k])
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

// validateSubtitleStyleFlags checks every style flag whose valid range is
// known without asking the backend.
//
// Three of these the backend clamps rather than refuses, which is the reason
// to check at all: ask for a subtitle at 1.5× the frame height and the request
// succeeds, having stored 0.98 instead. Nothing reports the substitution, so
// without this the caller's only evidence is the video looking wrong.
// Refusing states the range and changes nothing.
func validateSubtitleStyleFlags(cmd *cobra.Command) error {
	changed := func(name string) bool { return cmd.Flags().Changed(name) }

	if changed("subtitle-size") && flagSetSubtitleSize <= 0 {
		return clerr.Validation("--subtitle-size must be positive")
	}
	if changed("subtitle-font-weight") {
		w := flagSetSubtitleFontWeight
		if w < figlens.SubtitleFontWeightMin || w > figlens.SubtitleFontWeightMax {
			return clerr.Validationf("--subtitle-font-weight must be between %d and %d, got %d",
				figlens.SubtitleFontWeightMin, figlens.SubtitleFontWeightMax, w).
				WithHint("400 is regular and 700 is bold; a family that does not ship the weight you ask for falls back to one it has")
		}
	}
	if changed("subtitle-bottom") {
		v := flagSetSubtitleBottom
		if v < figlens.SubtitleBottomMin || v > figlens.SubtitleBottomMax {
			return clerr.Validationf("--subtitle-bottom must be between %g and %g, got %g",
				figlens.SubtitleBottomMin, figlens.SubtitleBottomMax, v).
				WithHint("it is a fraction of the frame height, not a pixel count or a percentage: 0.1 sits a tenth of the way up from the bottom")
		}
	}
	if changed("subtitle-stroke-width") {
		v := flagSetSubtitleStrokeWidth
		if v < 0 || v > figlens.SubtitleStrokeWidthMax {
			return clerr.Validationf("--subtitle-stroke-width must be between 0 and %g, got %g",
				figlens.SubtitleStrokeWidthMax, v).
				WithHint("0 removes the outline; an outline also needs --subtitle-bg-color transparent to be seen, which is why the presets are easier")
		}
	}
	if changed("subtitle-animation") {
		if _, ok := figlens.NormalizeSubtitleAnimation(flagSetSubtitleAnimation); !ok {
			return clerr.Validationf("--subtitle-animation %q is not one this CLI recognises", flagSetSubtitleAnimation).
				WithHintf("valid values: %s", strings.Join(figlens.SubtitleAnimations, ", "))
		}
	}
	return nil
}

// resolveSubtitlePreset turns what the caller typed into a preset. It accepts
// the display index from `vk subtitle presets` as well as the name, because
// the names are Chinese and contain a middle dot — correct to display, hostile
// to type, and worse to get through a shell.
func resolveSubtitlePreset(ctx context.Context, c *figlens.Client, v string) (figlens.SubtitlePreset, error) {
	want := strings.TrimSpace(v)
	if want == "" {
		return figlens.SubtitlePreset{}, clerr.Validation("--subtitle-preset cannot be empty").
			WithHint("run `vk subtitle presets` to see the looks, then pass a # or a name")
	}

	presets, err := c.ListSubtitlePresets(ctx)
	if err != nil {
		return figlens.SubtitlePreset{}, err
	}
	if len(presets) == 0 {
		return figlens.SubtitlePreset{}, clerr.Newf("the backend offers no subtitle presets").WithCode(6)
	}

	if n, err := strconv.Atoi(want); err == nil {
		if n < 1 || n > len(presets) {
			return figlens.SubtitlePreset{}, clerr.Validationf("no subtitle preset #%d", n).
				WithHintf("there are %d, numbered 1–%d; run `vk subtitle presets` to see them", len(presets), len(presets))
		}
		return presets[n-1], nil
	}
	for _, p := range presets {
		if strings.EqualFold(p.Name, want) {
			return p, nil
		}
	}
	names := make([]string, 0, len(presets))
	for _, p := range presets {
		names = append(names, p.Name)
	}
	return figlens.SubtitlePreset{}, clerr.Validationf("no subtitle preset named %q", want).
		WithHintf("the %d available are: %s (or pass the # from `vk subtitle presets`)", len(presets), strings.Join(names, ", "))
}

// resolveSubtitleFont turns what the caller typed into a font family, taking
// the display index from `vk subtitle fonts` as well as the family itself.
//
// The check is worth a round trip: the backend's refusal is a flat
// "fontFamily not allowed" that names neither what is allowed nor how to find
// out, and a caller with no list will guess — reasonably, and wrongly, since
// the families are exact strings like "LXGW WenKai".
func resolveSubtitleFont(ctx context.Context, c *figlens.Client, v string) (string, error) {
	want := strings.TrimSpace(v)
	if want == "" {
		return "", clerr.Validation("--subtitle-font cannot be empty").
			WithHint("run `vk subtitle fonts` to see the families, then pass a # or a family")
	}

	fonts, err := c.ListSubtitleFonts(ctx)
	if err != nil {
		return "", err
	}
	if len(fonts) == 0 {
		return "", clerr.Newf("the backend offers no subtitle fonts").WithCode(6)
	}

	if n, err := strconv.Atoi(want); err == nil {
		if n < 1 || n > len(fonts) {
			return "", clerr.Validationf("no subtitle font #%d", n).
				WithHintf("there are %d, numbered 1–%d; run `vk subtitle fonts` to see them", len(fonts), len(fonts))
		}
		return fonts[n-1].Family, nil
	}
	for _, f := range fonts {
		if strings.EqualFold(f.Family, want) {
			// Return the catalog's spelling, not the caller's: the backend
			// compares exactly, so a family matched case-insensitively here
			// would still be refused on the wire.
			return f.Family, nil
		}
	}
	return "", clerr.Validationf("no subtitle font named %q", want).
		WithHintf("run `vk subtitle fonts` to see the %d families that are allowed, and pass a # or an exact family", len(fonts))
}

// describeSubtitleStyle renders a style as one named field per line, listing
// exactly the fields that will be stored.
//
// The default struct formatting prints a bare list of values — "{Source Han
// Sans 36 0 #ffffff rgba(8,8,12,0.68) 0 #000000 0 fade}" — which names
// nothing and pads the gaps with zeroes that were never set. It was tolerable
// while a change touched one field; a preset sets five or six at once, and the
// whole point of showing the result is that the caller can check it.
//
// Zero-valued fields are omitted rather than shown as 0, because that is what
// goes on the wire: the payload omits them, and the renderer reads missing as
// its own default. Printing "strokeWidth = 0" would claim a setting that is
// not being stored.
func describeSubtitleStyle(s figlens.SubtitleStyle) []string {
	var lines []string
	add := func(name string, v any, zero bool) {
		if !zero {
			lines = append(lines, fmt.Sprintf("%s = %v", name, v))
		}
	}
	add("fontFamily", s.FontFamily, s.FontFamily == "")
	add("fontSize", s.FontSize, s.FontSize == 0)
	add("fontWeight", s.FontWeight, s.FontWeight == 0)
	add("color", s.Color, s.Color == "")
	add("backgroundColor", s.BackgroundColor, s.BackgroundColor == "")
	add("bottomPercent", s.BottomPercent, s.BottomPercent == 0)
	add("strokeColor", s.StrokeColor, s.StrokeColor == "")
	add("strokeWidth", s.StrokeWidth, s.StrokeWidth == 0)
	add("animation", s.Animation, s.Animation == "")
	if len(lines) == 0 {
		return []string{"(cleared — the renderer's defaults apply)"}
	}
	return lines
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
	// No backticks in these descriptions: cobra reads the first backquoted
	// span as the flag's argument placeholder, so "from `vk subtitle fonts`"
	// would render the usage line as `--subtitle-font vk subtitle fonts`
	// instead of `--subtitle-font string`.
	setCmd.Flags().StringVar(&flagSetSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	setCmd.Flags().StringVar(&flagSetTitle, "title", "", "rename the task (changes no rendered output)")
	setCmd.Flags().StringVar(&flagSetBGM, "bgm", "", "background music: on or off")
	setCmd.Flags().Float64Var(&flagSetBGMVolume, "bgm-volume", 0, "background music level, 0.1–2.0 (1.0 = unchanged)")
	setCmd.Flags().StringVar(&flagSetSubtitle, "subtitle", "", "subtitles: on or off")
	setCmd.Flags().StringVar(&flagSetSubtitlePreset, "subtitle-preset", "", "ready-made look from 'vk subtitle presets' — either the # or the name; other --subtitle-* flags apply on top")
	setCmd.Flags().IntVar(&flagSetSubtitleSize, "subtitle-size", 0, "subtitle font size in px")
	setCmd.Flags().StringVar(&flagSetSubtitleColor, "subtitle-color", "", "subtitle text colour, e.g. #FFFFFF")
	setCmd.Flags().StringVar(&flagSetSubtitleFont, "subtitle-font", "", "font family from 'vk subtitle fonts' — either the # or the exact family")
	setCmd.Flags().IntVar(&flagSetSubtitleFontWeight, "subtitle-font-weight", 0, "subtitle font weight, 100–900 (400 regular, 700 bold)")
	setCmd.Flags().StringVar(&flagSetSubtitleBgColor, "subtitle-bg-color", "", "colour of the plate behind the subtitle; 'transparent' for none")
	setCmd.Flags().Float64Var(&flagSetSubtitleBottom, "subtitle-bottom", 0, "subtitle position above the bottom edge, as a fraction of frame height, 0.02–0.98")
	setCmd.Flags().StringVar(&flagSetSubtitleStrokeColor, "subtitle-stroke-color", "", "subtitle outline colour; only visible with a stroke width above 0")
	setCmd.Flags().Float64Var(&flagSetSubtitleStrokeWidth, "subtitle-stroke-width", 0, "subtitle outline width in px, 0–12 (0 = no outline)")
	setCmd.Flags().StringVar(&flagSetSubtitleAnimation, "subtitle-animation", "",
		"per-line entry animation; one of "+strings.Join(figlens.SubtitleAnimations, ", "))
}
