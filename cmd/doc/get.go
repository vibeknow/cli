package doc

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
)

var flagGetKBID string

var getCmd = &cobra.Command{
	Use:   "get <doc_id>",
	Short: "fetch document status from vectoria",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docID := args[0]
		if flagGetKBID == "" {
			return clerr.Validation("--kb-id is required")
		}

		c, err := cliauth.NewVectoriaClient()
		if err != nil {
			return err
		}
		d, err := c.GetDocStatus(context.Background(), flagGetKBID, docID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			payload := map[string]any{
				"doc_id": d.ID,
				"status": d.Status,
			}
			if d.Error != "" {
				payload["error"] = d.Error
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(payload)
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
