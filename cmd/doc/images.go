package doc

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/output"
)

var flagImagesKBID string

var imagesCmd = &cobra.Command{
	Use:   "images <doc_id>",
	Short: "list candidate document images for --images selection",
	Long: `images extracts the candidate images of a parsed document (idempotent)
and prints their image_index values. Pass the indexes you want embedded to
` + "`vk create --images 1,3,5`" + ` (any mode except replica).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docID := args[0]
		if flagImagesKBID == "" {
			return clerr.Validation("--kb-id is required")
		}

		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}
		fc := figlens.New(url, tp)

		images, err := fc.ExtractDocImages(context.Background(), flagImagesKBID, docID, "")
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			items := make([]map[string]any, 0, len(images))
			for _, img := range images {
				item := map[string]any{
					"image_index": img.ImageIndex,
					"url":         img.URL,
					"description": img.Description,
					"type":        img.Type,
				}
				if img.Context != "" {
					item["context"] = img.Context
				}
				items = append(items, item)
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"doc_id": docID,
				"images": items,
			})
		}

		if len(images) == 0 {
			fmt.Println("no candidate images found")
			return nil
		}
		for _, img := range images {
			desc := img.Description
			if desc == "" {
				desc = "-"
			}
			fmt.Printf("[%d] %s\n    %s\n", img.ImageIndex, desc, img.URL)
		}
		fmt.Printf("\nuse: vk create --from %s --kb-id %s --images <i,j,...>\n", docID, flagImagesKBID)
		return nil
	},
}

func init() {
	imagesCmd.Flags().StringVar(&flagImagesKBID, "kb-id", "", "knowledge base ID (required)")
}
