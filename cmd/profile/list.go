package profile

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if len(f.Profiles) == 0 {
			fmt.Fprintln(os.Stdout, "(no profiles)")
			return nil
		}
		for _, p := range f.Profiles {
			marker := "  "
			if p.Name == f.Current {
				marker = "* "
			}
			fmt.Printf("%s%s\t%s\n", marker, p.Name, p.APIEndpoint)
		}
		return nil
	},
}
