package subtitle

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var presetsCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "presets",
	Short: "list the ready-made subtitle looks",
	Long: `The subtitle looks the product ships, for ` + "`vk video set --subtitle-preset`" + `.

Prefer one of these over setting colours and outlines by hand. A readable
subtitle is a combination, not a set of independent fields: the outlined looks
also clear the background plate, and the plated looks also switch the outline
off. Set one without the other and you get a subtitle with both, or with
neither.

A preset only sets the fields that make up its look. Font size, vertical
position and entry animation belong to the video rather than to the look, so a
preset leaves whatever the work already has, and ` + "`--subtitle-size`" + ` and friends
still apply on top.`,
	Example: `  vk subtitle presets
  vk subtitle presets --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}

		presets, err := figlens.New(url, tp).ListSubtitlePresets(context.Background())
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(presets))
			for i, p := range presets {
				// The patch ships whole, not summarised: it is the only way
				// a caller can tell what applying this will and will not
				// disturb, and the omitted fields carry as much meaning as
				// the present ones.
				items = append(items, map[string]any{
					"n":     i + 1,
					"name":  p.Name,
					"patch": p.Patch,
					"sets":  patchFields(p.Patch),
				})
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"presets": items,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "#\tNAME\tLOOK")
		for i, p := range presets {
			fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, p.Name, describePatch(p.Patch))
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("subtitle.presets.hint"))
		return nil
	},
}

// describePatch renders a look in words. The names are the design team's and
// say how a preset was meant to be used, not what it does to the frame — and
// half of them name a font rather than a look at all. Without this column,
// picking one from a terminal means applying it to find out.
func describePatch(p figlens.SubtitleStylePatch) string {
	var parts []string
	if p.Color != nil {
		parts = append(parts, "text "+*p.Color)
	}
	if p.BackgroundColor != nil {
		if isTransparent(*p.BackgroundColor) {
			parts = append(parts, "no plate")
		} else {
			parts = append(parts, "plate "+*p.BackgroundColor)
		}
	}
	if p.StrokeWidth != nil {
		if *p.StrokeWidth == 0 {
			parts = append(parts, "no outline")
		} else {
			outline := "outline " + strconv.FormatFloat(*p.StrokeWidth, 'g', -1, 64) + "px"
			if p.StrokeColor != nil {
				outline += " " + *p.StrokeColor
			}
			parts = append(parts, outline)
		}
	}
	if p.FontFamily != nil {
		font := *p.FontFamily
		if p.FontWeight != nil {
			font += " " + strconv.Itoa(*p.FontWeight)
		}
		parts = append(parts, font)
	}
	if len(parts) == 0 {
		return "(sets nothing)"
	}
	return strings.Join(parts, " · ")
}

// isTransparent recognises the one no-plate spelling the catalog uses, plus
// the empty string. It is deliberately not a colour parser: anything it does
// not recognise is shown verbatim, which is honest, rather than guessed at.
func isTransparent(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "" || s == "transparent"
}

// patchFields lists the style fields a preset sets, so a caller can see what
// applying it will overwrite without inspecting the patch field by field.
func patchFields(p figlens.SubtitleStylePatch) []string {
	fields := make([]string, 0, 9)
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"fontFamily", p.FontFamily != nil},
		{"fontSize", p.FontSize != nil},
		{"fontWeight", p.FontWeight != nil},
		{"color", p.Color != nil},
		{"backgroundColor", p.BackgroundColor != nil},
		{"bottomPercent", p.BottomPercent != nil},
		{"strokeColor", p.StrokeColor != nil},
		{"strokeWidth", p.StrokeWidth != nil},
		{"animation", p.Animation != nil},
	} {
		if f.set {
			fields = append(fields, f.name)
		}
	}
	return fields
}
