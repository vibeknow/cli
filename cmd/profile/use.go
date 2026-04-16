package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var useCmd = &cobra.Command{
	Use:   "use NAME",
	Short: "switch active profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.UseProfile(args[0]); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.switched", args[0]))
		return nil
	},
}
