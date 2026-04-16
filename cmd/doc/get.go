package doc

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/client/vectoria"
	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
)

var flagGetKBID string

var getCmd = &cobra.Command{
	Use:   "get <doc_id>",
	Short: "fetch document status from vectoria",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docID := args[0]
		if flagGetKBID == "" {
			return fmt.Errorf("--kb-id is required")
		}

		apiKey := os.Getenv("VECTORIA_API_KEY")

		url, err := endpoints.Resolve(config.Profile{}, "vectoria")
		if err != nil {
			return err
		}

		c := vectoria.New(url, apiKey)
		d, err := c.GetDocStatus(context.Background(), flagGetKBID, docID)
		if err != nil {
			return err
		}

		fmt.Printf("doc_id=%s\nstatus=%s\n", d.ID, d.Status)
		if d.Error != "" {
			fmt.Printf("error=%s\n", d.Error)
		}
		return nil
	},
}

func init() {
	getCmd.Flags().StringVar(&flagGetKBID, "kb-id", "", "knowledge base ID (required)")
}
