package doc

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "upload a document to vectoria (creates KB, polls until completed)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		fi, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", filePath, err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", filePath)
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
		ctx := context.Background()

		kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
		fmt.Fprintf(os.Stderr, "creating knowledge base %q...\n", kbName)
		kbID, err := c.CreateKB(ctx, kbName)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "kb_id: %s\n", kbID)

		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open %q: %w", filePath, err)
		}
		defer f.Close()

		fmt.Fprintf(os.Stderr, "uploading %q...\n", fi.Name())
		doc, err := c.UploadDoc(ctx, kbID, fi.Name(), f)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "doc_id: %s — polling for completion...\n", doc.ID)

		deadline := time.Now().Add(10 * time.Minute)
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("timed out waiting for document processing (10m)")
			}

			d, err := c.GetDocStatus(ctx, kbID, doc.ID)
			if err != nil {
				return err
			}

			switch d.Status {
			case "completed":
				fmt.Printf("kb_id=%s\ndoc_id=%s\n", kbID, d.ID)
				return nil
			case "failed", "error":
				return fmt.Errorf("document processing failed: %s", d.Error)
			default:
				fmt.Fprintf(os.Stderr, "status: %s\n", d.Status)
				time.Sleep(2 * time.Second)
			}
		}
	},
}
