package video

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "video",
	Short: "manage video tasks and exports",
}

func init() {
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(waitCmd)
	Cmd.AddCommand(downloadCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(urlCmd)
	Cmd.AddCommand(exportCmd)
	Cmd.AddCommand(exportStatusCmd)
}
