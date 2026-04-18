package voice

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cmdutil"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list available voice templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("vibeknow")
		if err != nil {
			return err
		}

		c := vibeknow.New(url, tp)
		templates, err := c.ListVoiceTemplates(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tCATEGORY\tSPEECH_VOICE_ID")
		for _, t := range templates {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", t.ID, t.Name, t.Category, t.SpeechVoiceID)
		}
		return w.Flush()
	},
}
