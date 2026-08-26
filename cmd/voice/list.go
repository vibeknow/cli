package voice

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/output"
)

var flagListLanguage string

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list voices: public templates grouped by language, plus your cloned voices",
	Example: `  vk voice list
  vk voice list --language en-US
  vk voice list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, url, tp, err := cmdutil.Default().Service("vibeknow")
		if err != nil {
			return err
		}

		c := vibeknow.New(url, tp)
		catalog, err := c.ListPipelineVoices(context.Background())
		if err != nil {
			return err
		}

		langFilter := strings.TrimSpace(flagListLanguage)
		if langFilter != "" {
			var kept []vibeknow.LanguageVoices
			for _, g := range catalog.Languages {
				if strings.EqualFold(g.Language, langFilter) {
					kept = append(kept, g)
				}
			}
			catalog.Languages = kept
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			toItem := func(t vibeknow.VoiceTemplate) map[string]any {
				item := map[string]any{
					"id":              t.ID,
					"name":            t.Name,
					"category":        t.Category,
					"speech_voice_id": t.SpeechVoiceID,
				}
				if t.Language != "" {
					item["language"] = t.Language
				}
				return item
			}
			// `templates` keeps the pre-catalog flat shape (now with a
			// language field) so existing consumers keep parsing; the
			// grouped view and cloned voices ride alongside.
			//
			// It is built from Flatten so it means one thing: every voice
			// `--voice` accepts, cloned ones included. Listing only the
			// public presets here would hide a user's own cloned voice from
			// the flat view that the docs point agents at, while the human
			// table lists it — the same command answering "what can I use?"
			// two different ways depending on --output.
			templates := make([]map[string]any, 0)
			for _, t := range catalog.Flatten() {
				templates = append(templates, toItem(t))
			}
			languages := make([]map[string]any, 0, len(catalog.Languages))
			for _, g := range catalog.Languages {
				voices := make([]map[string]any, 0, len(g.Voices))
				for _, t := range g.Voices {
					voices = append(voices, toItem(t))
				}
				languages = append(languages, map[string]any{
					"language": g.Language,
					"voices":   voices,
				})
			}
			cloned := make([]map[string]any, 0, len(catalog.Cloned))
			for _, t := range catalog.Cloned {
				cloned = append(cloned, toItem(t))
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"templates": templates,
				"languages": languages,
				"cloned":    cloned,
			})
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		// The first column is headed "#", not "ID": it is a display index,
		// while SPEECH_VOICE_ID is the identifier the backend's TTS keys on.
		// `--voice` accepts either, but the header should say which is real.
		fmt.Fprintln(w, "#\tNAME\tLANGUAGE\tCATEGORY\tSPEECH_VOICE_ID")
		for _, g := range catalog.Languages {
			for _, t := range g.Voices {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Name, g.Language, t.Category, t.SpeechVoiceID)
			}
		}
		// Cloned voices are language-independent: usable with any --language.
		for _, t := range catalog.Cloned {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Name, "(cloned, any)", t.Category, t.SpeechVoiceID)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "\npass either column to `vk create --voice` (e.g. --voice 1 or --voice <SPEECH_VOICE_ID>); pick a voice matching your --language")
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&flagListLanguage, "language", "", "only show public voices for this locale (e.g. zh-CN, en-US); cloned voices always show")
}
