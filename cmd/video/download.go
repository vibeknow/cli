package video

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagDownloadSessionID string
	flagDownloadOutput    string
	flagDownloadOverwrite bool
)

var downloadCmd = &cobra.Command{
	Use:   "download <task_id>",
	Short: "download the rendered video for a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDownloadSessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := context.Background()

		// Check if work already has a video_path.
		w, err := c.GetWorkBySession(ctx, flagDownloadSessionID)
		if err != nil {
			return err
		}

		videoPath := w.VideoPath
		if videoPath == "" {
			// Submit export and poll.
			fmt.Fprintf(os.Stderr, "submitting export...\n")
			exportTaskID, err := c.ExportVideo(ctx, flagDownloadSessionID)
			if err != nil {
				return err
			}

			deadline := time.Now().Add(10 * time.Minute)
			for {
				if time.Now().After(deadline) {
					return fmt.Errorf("timed out waiting for export (10m)")
				}

				result, err := c.GetExportResult(ctx, exportTaskID)
				if err != nil {
					return err
				}

				switch result.Status {
				case "completed", "success":
					videoPath = result.VideoPath
					if videoPath == "" {
						return fmt.Errorf("export completed but no video_path returned")
					}
				case "failed", "error":
					return fmt.Errorf("export failed")
				default:
					fmt.Fprintf(os.Stderr, "export status: %s\n", result.Status)
					time.Sleep(3 * time.Second)
					continue
				}

				break
			}
		}

		// Resolve signed URL.
		signedURL, err := c.SignedURL(ctx, videoPath)
		if err != nil {
			return err
		}

		// Determine output file name.
		outPath := flagDownloadOutput
		if outPath == "" {
			outPath = flagDownloadSessionID + ".mp4"
		}

		if !flagDownloadOverwrite {
			if _, err := os.Stat(outPath); err == nil {
				return fmt.Errorf("file %q already exists; use --overwrite to replace", outPath)
			}
		}

		// Download.
		fmt.Fprintf(os.Stderr, "downloading to %q...\n", outPath)
		if err := downloadFile(signedURL, outPath); err != nil {
			return err
		}
		fmt.Printf("output=%s\n", outPath)
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
