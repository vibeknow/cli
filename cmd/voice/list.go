package voice

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/client/vibeknow"
	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list available voice templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		tok, _, err := cliauth.ResolverFor(p).Resolve()
		if err != nil {
			return fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
		}
		url, err := endpoints.Resolve(p, "vibeknow")
		if err != nil {
			return err
		}

		c := vibeknow.New(url, staticToken(tok))
		templates, err := c.ListVoiceTemplates(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tLANGUAGE\tGENDER")
		for _, t := range templates {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Language, t.Gender)
		}
		return w.Flush()
	},
}

type staticToken string

func (s staticToken) Token(_ context.Context) (string, error) { return string(s), nil }
