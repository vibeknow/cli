package voice

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/output"
)

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
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

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(templates))
			for _, t := range templates {
				items = append(items, map[string]any{
					"id":              t.ID,
					"name":            t.Name,
					"category":        t.Category,
					"speech_voice_id": t.SpeechVoiceID,
				})
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"templates": items,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		// The first column is headed "#", not "ID": it is a display index,
		// while SPEECH_VOICE_ID is the identifier the backend's TTS keys on.
		// Heading it "ID" invited users to pass it to `--voice`, which the
		// backend then rejected deep inside the TTS node. `--voice` accepts
		// either now, but the header should still say which one is real.
		fmt.Fprintln(w, "#\tNAME\tCATEGORY\tSPEECH_VOICE_ID")
		for _, t := range templates {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", t.ID, t.Name, t.Category, t.SpeechVoiceID)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\npass either column to `vk create --voice` (e.g. --voice 1 or --voice <SPEECH_VOICE_ID>)")
		return nil
	},
}
