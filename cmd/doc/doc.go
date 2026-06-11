package doc

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "doc",
	Short: "manage documents in vectoria",
}

func init() {
	Cmd.AddCommand(uploadCmd)
	Cmd.AddCommand(getCmd)
	Cmd.AddCommand(imagesCmd)
}
