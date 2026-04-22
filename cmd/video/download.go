package video

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/validate"
)

var (
	flagDownloadSessionID string
	flagDownloadOutput    string
	flagDownloadOverwrite bool
)

var downloadCmd = &cobra.Command{
	Use:   "download <task_id>",
	Short: "download the rendered MP4 for a task (export must already be complete)",
	Args:  cobra.ExactArgs(1),
	Example: `  vk video download 123 --session-id sess_xxx
  vk video download 123 --session-id sess_xxx --output out.mp4 --overwrite`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDownloadSessionID == "" {
			return clerr.Validation("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := context.Background()
		w, err := c.GetWorkBySession(ctx, flagDownloadSessionID)
		if err != nil {
			return err
		}
		if w.VideoPath == "" {
			return clerr.Validation(i18n.T("download.not_exported")).
				WithHint(i18n.T("download.not_exported.hint", args[0], flagDownloadSessionID))
		}

		signedURL, err := c.SignedURL(ctx, w.VideoPath)
		if err != nil {
			return err
		}

		rawOut := flagDownloadOutput
		if rawOut == "" {
			rawOut = flagDownloadSessionID + ".mp4"
		}
		outPath, err := validate.SafeOutputPath(rawOut)
		if err != nil {
			return clerr.Validation(err.Error()).
				WithHint("--output must be a relative path inside the current directory")
		}

		if !flagDownloadOverwrite {
			if _, err := os.Stat(outPath); err == nil {
				return clerr.Validationf("file %q already exists; use --overwrite to replace", rawOut)
			}
		}

		fmt.Fprintf(os.Stderr, "downloading to %q...\n", rawOut)
		if err := downloadFile(signedURL, outPath); err != nil {
			return err
		}
		fmt.Printf("output=%s\n", rawOut)
		return nil
	},
}

func downloadFile(url, dest string) (retErr error) {
	f, err := os.CreateTemp("", "vibeknow-download-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()

	defer func() {
		if retErr != nil {
			f.Close()
			os.Remove(tmpPath)
		}
	}()

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("rename to %q: %w", dest, err)
	}
	return nil
}

func init() {
	downloadCmd.Flags().StringVar(&flagDownloadSessionID, "session-id", "", "session ID (required)")
	downloadCmd.Flags().StringVar(&flagDownloadOutput, "output", "", "output file path (default: <session_id>.mp4)")
	downloadCmd.Flags().BoolVar(&flagDownloadOverwrite, "overwrite", false, "overwrite existing output file")
}
