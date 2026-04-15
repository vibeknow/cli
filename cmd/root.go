package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:          "vibeknow",
	Short:        "vibeknow CLI — turn docs into videos",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}
