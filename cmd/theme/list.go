package theme

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var flagListMode string

// suiteForMode maps `vk create --mode` vocabulary onto the backend's theme
// suite: each mode draws from exactly one catalog, and passing a theme from
// the wrong suite is a hard 400 at the stream entry — so the list command
// speaks modes, not suite names, and the user can't ask a mismatched
// question. replica shares the standard line's design catalog.
func suiteForMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default", "replica":
		return figlens.ThemeSuiteDesign, nil
	case "image":
		return figlens.ThemeSuiteImage2, nil
	case "handdraw":
		return figlens.ThemeSuiteHandDraw, nil
	default:
		return "", clerr.Validation(i18n.T("theme.err.mode_invalid", mode))
	}
}

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list themes for a creation mode",
	Example: `  vk theme list
  vk theme list --mode image
  vk theme list --mode handdraw --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		suite, err := suiteForMode(flagListMode)
		if err != nil {
			return err
		}

		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}

		themes, err := figlens.New(url, tp).ListThemes(context.Background(), suite)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(themes))
			for _, t := range themes {
				item := map[string]any{
					"id":   t.ID,
					"name": t.Name,
					"desc": t.Desc,
					"tags": t.Tags,
				}
				if t.Badge != "" {
					item["badge"] = t.Badge
				}
				if t.Preview != nil {
					item["preview"] = map[string]string{
						"webp": t.Preview.Webp, "poster": t.Preview.Poster,
						"webp_vertical": t.Preview.WebpV, "poster_vertical": t.Preview.PosterV,
					}
				}
				items = append(items, item)
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"mode":   flagListMode,
				"themes": items,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTAGS\tDESC")
		for _, t := range themes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.ID, t.Name, strings.Join(t.Tags, ","), t.Desc)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("theme.list.hint"))
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&flagListMode, "mode", "", i18n.T("theme.flag.mode"))
}
