package ip

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/charcheck"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list IP characters: public ones plus your own trained ones",
	Long: `List the characters an image-mode video (图解视频) can feature, for
` + "`vk create --mode image --ip-asset <REF>`" + `.

An IP character is who appears in the illustrations; the theme is how they
are drawn. The two are chosen independently. Pass --ip-asset none to leave
out the character your account uses by default.

Your own characters are trained on the web. Only usable ones are listed.`,
	Example: `  vk ip list
  vk ip list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}
		items, err := figlens.New(url, tp).ListIPAssets(context.Background())
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == output.FormatJSON || format == output.FormatNDJSON {
			out := make([]map[string]any, 0, len(items))
			for _, it := range items {
				item := map[string]any{
					"ref":       it.Ref,
					"name":      it.Name,
					"image_url": it.ImageURL,
					"source":    source(it),
				}
				if it.Subtitle != "" {
					item["subtitle"] = it.Subtitle
				}
				if it.Enabled == 1 {
					// Featured when a run names no IP; --ip-asset none opts out.
					item["account_default"] = true
				}
				out = append(out, item)
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{"ips": out})
		}

		w := tabwriter.NewWriter(charcheck.NewWriter(os.Stdout), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REF\tNAME\tSOURCE\tNOTE")
		for _, it := range items {
			note := it.Subtitle
			if it.Enabled == 1 {
				note = "(account default) " + note
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", it.Ref, it.Name, source(it), note)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("ip.list.hint"))
		return nil
	},
}

func source(it figlens.IPAsset) string {
	if it.Public {
		return "public"
	}
	return "custom"
}
