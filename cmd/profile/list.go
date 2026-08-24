package profile

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		items := make([]map[string]any, 0, len(f.Profiles))
		for _, p := range f.Profiles {
			items = append(items, map[string]any{
				"name":          p.Name,
				"current":       p.Name == f.Current,
				"api_endpoint":  p.APIEndpoint,
				"trust":         p.Trust,
				"is_production": p.IsProduction,
			})
		}
		return cmdutil.Emit(cmd, map[string]any{
			"profiles": items,
			"current":  f.Current,
			"total":    len(items),
		}, "profile.list", func(w io.Writer) {
			if len(f.Profiles) == 0 {
				fmt.Fprintln(w, "(no profiles)")
				return
			}
			for _, p := range f.Profiles {
				marker := "  "
				if p.Name == f.Current {
					marker = "* "
				}
				fmt.Fprintf(w, "%s%s\t%s\n", marker, p.Name, p.APIEndpoint)
			}
		})
	},
}
