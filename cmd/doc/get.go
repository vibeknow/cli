package doc

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
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
		if apiKey == "" {
			return clerr.New("文档服务认证失败").WithHint("请设置 VECTORIA_API_KEY 环境变量")
		}

		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, "vectoria")
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
