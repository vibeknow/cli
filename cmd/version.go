package cmd

import (
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print CLI version",
	RunE: func(cmd *cobra.Command, args []string) error {
		// os/arch travel with the version because the first thing anyone
		// asks about a bug report is which build produced it.
		return cmdutil.Emit(cmd, map[string]any{
			"version":    version,
			"go_version": runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
		}, "version", func(w io.Writer) {
			fmt.Fprintln(w, version)
		})
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
