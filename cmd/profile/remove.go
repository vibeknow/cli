package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/i18n"
)

var removeCmd = &cobra.Command{
	Use:   "remove NAME",
	Short: "delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.RemoveProfile(args[0]); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.removed", args[0]))
		return nil
	},
}
