package doc

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "upload a document to vectoria (creates KB, polls until completed)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		// Progress narration goes to stderr throughout; stdout carries only
		// the result, so `vk doc upload x.pdf --output json | jq` works.
		stderr := cmd.ErrOrStderr()

		fi, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", filePath, err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", filePath)
		}

		c, err := cliauth.NewVectoriaClient()
		if err != nil {
			return err
		}
		ctx := context.Background()

		kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
		fmt.Fprintf(stderr, "creating knowledge base %q...\n", kbName)
		kbID, err := c.CreateKB(ctx, kbName)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "kb_id: %s\n", kbID)

		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open %q: %w", filePath, err)
		}
		defer f.Close()

		fmt.Fprintf(stderr, "uploading %q...\n", fi.Name())
		doc, err := c.UploadDoc(ctx, kbID, fi.Name(), f)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "doc_id: %s — polling for completion...\n", doc.ID)

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
				return cmdutil.Emit(cmd, map[string]any{
					"kb_id":    kbID,
					"doc_id":   d.ID,
					"filename": fi.Name(),
				}, "doc.uploaded", func(w io.Writer) {
					fmt.Fprintf(w, "kb_id=%s\ndoc_id=%s\n", kbID, d.ID)
				})
			case "failed", "error":
				return fmt.Errorf("document processing failed: %s", d.Error)
			default:
				fmt.Fprintf(stderr, "status: %s\n", d.Status)
				time.Sleep(2 * time.Second)
			}
		}
	},
}
