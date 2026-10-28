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
	"github.com/vibeknow/cli/internal/modes"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/themecat"
)

var flagListMode string

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list themes for a creation mode",
	Long: `List every value ` + "`vk create --theme`" + ` accepts in one mode: your own
custom themes (SOURCE=custom, ua_<id>) first, then the official catalog.

--mode takes the same values as ` + "`vk create --mode`" + `, and the list is exactly
what create validates against: an id from here passes, anything else is
refused before upload. Themes, styles and templates (主题 / 风格 / 模板) are
all this one thing.`,
	Example: `  vk theme list
  vk theme list --mode image
  vk theme list --mode handdraw --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Same table create reads, so a name valid here is valid there.
		m, ok := modes.Resolve(flagListMode)
		if !ok {
			return clerr.Validation(i18n.T("theme.err.mode_invalid", modes.Names(), flagListMode))
		}

		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}

		themes, err := themecat.List(context.Background(), figlens.New(url, tp), m)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(themes))
			for _, t := range themes {
				tags := t.Tags
				if tags == nil {
					tags = []string{}
				}
				item := map[string]any{
					"id":     t.ID,
					"name":   t.Name,
					"desc":   t.Desc,
					"tags":   tags,
					"source": t.Source,
				}
				if t.Base != "" {
					item["base"] = t.Base
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
				"mode":   m.Name,
				"themes": items,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSOURCE\tTAGS\tDESC")
		for _, t := range themes {
			desc := t.Desc
			if t.Base != "" {
				desc = "base: " + t.Base
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Source, strings.Join(t.Tags, ","), desc)
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
