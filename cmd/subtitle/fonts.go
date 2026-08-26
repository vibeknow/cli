package subtitle

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var fontsCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "fonts",
	Short: "list the font families subtitles can use",
	Long: `Every family ` + "`vk video set --subtitle-font`" + ` will accept.

This is the whole set, not a sample: the backend validates against the same
catalog it serves here, so a family in this list is guaranteed to be accepted
and one that is not will be refused. Display faces that are unreadable at
subtitle size are already filtered out.

The catalog knows which numeric weights each family ships, but does not
publish them, so ` + "`--subtitle-font-weight`" + ` cannot be checked against the
family you picked. Asking for a weight a family lacks is not an error — the
renderer falls back to what it has.`,
	Example: `  vk subtitle fonts
  vk subtitle fonts --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}

		fonts, err := figlens.New(url, tp).ListSubtitleFonts(context.Background())
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(fonts))
			for i, f := range fonts {
				items = append(items, map[string]any{
					"n":      i + 1,
					"family": f.Family,
					"label":  f.Label,
				})
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"fonts": items,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		// "#" is a display index, "FAMILY" is the value the backend stores.
		// --subtitle-font takes either; the header should say which is real.
		fmt.Fprintln(w, "#\tFAMILY\tLABEL")
		for i, f := range fonts {
			fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, f.Family, f.Label)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("subtitle.fonts.hint"))
		return nil
	},
}
