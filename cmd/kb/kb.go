// Package kb implements the `vk kb` subcommand family: list, delete, prune.
package kb

import (
	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/i18n"
)

var Cmd = &cobra.Command{
	Use:   "kb",
	Short: i18n.T("kb.short"),
}
