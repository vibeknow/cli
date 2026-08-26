package avatar

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list avatars: public presets plus your own trained ones",
	Example: `  vk avatar list
  vk avatar list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}
		c := figlens.New(url, tp)
		ctx := context.Background()

		catalog, err := c.ListAvatarCatalog(ctx)
		if err != nil {
			return err
		}
		// Own avatars are additive: a catalog that loads without them is
		// still useful, so their failure degrades to a stderr note rather
		// than sinking the whole listing.
		mine, mineErr := c.ListMyAvatars(ctx)
		if mineErr != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("avatar.list.mine_failed", mineErr))
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			pub := make([]map[string]any, 0, len(catalog))
			for _, a := range catalog {
				item := map[string]any{
					"id":   a.ID,
					"name": a.Name,
				}
				if a.Style != "" {
					item["style"] = a.Style
				}
				if a.Gender != "" {
					item["gender"] = a.Gender
				}
				if a.VoiceID != "" {
					item["voice_id"] = a.VoiceID
				}
				if len(a.Tags) > 0 {
					item["tags"] = a.Tags
				}
				if a.MemberOnly {
					item["member_only"] = true
				}
				pub = append(pub, item)
			}
			own := make([]map[string]any, 0, len(mine))
			for _, a := range mine {
				own = append(own, map[string]any{
					"id":     fmt.Sprintf("%s%d", figlens.AvatarRefUserPrefix, a.ID),
					"name":   a.Name,
					"status": figlens.AvatarStatusLabel(a.Status),
					"usable": a.Status == figlens.UserAssetStatusActive,
				})
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"public": pub,
				"mine":   own,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTYLE\tGENDER\tVOICE_ID\tNOTE")
		for _, a := range catalog {
			note := strings.Join(a.Tags, ",")
			if a.MemberOnly {
				if note != "" {
					note += " "
				}
				note += "(member-only)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Style, a.Gender, a.VoiceID, note)
		}
		for _, a := range mine {
			note := figlens.AvatarStatusLabel(a.Status)
			if a.Status != figlens.UserAssetStatusActive {
				note += " (not usable yet)"
			}
			fmt.Fprintf(w, "%s%d\t%s\t\t\t\t%s\n", figlens.AvatarRefUserPrefix, a.ID, a.Name, note)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\n"+i18n.T("avatar.list.hint"))
		return nil
	},
}
