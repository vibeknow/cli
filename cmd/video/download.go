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
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/validate"
)

var (
	flagDownloadSessionID string
	flagDownloadDest      string
	flagDownloadOverwrite bool
)

var downloadCmd = &cobra.Command{
	Use:   "download [task_id]",
	Short: "download the rendered MP4 for a task (export must already be complete)",
	Args:  cobra.MaximumNArgs(1),
	Long: `download fetches the exported MP4 for a task to a local file.

The destination is --dest. Until 0.8 this command used --output for the
file path, which shadowed the global --output format flag and made
` + "`vk video download --output json`" + ` write a file literally named "json".
--output now means format here as it does everywhere else.`,
	Example: `  vk video download --session-id sess_xxx
  vk video download 123 --session-id sess_xxx --dest out.mp4 --overwrite
  vk video download 123 --session-id sess_xxx --dest out.mp4 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, sessionID, err := resolveTarget(args, flagDownloadSessionID)
		if err != nil {
			return err
		}
		flagDownloadSessionID = sessionID

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		w, err := c.GetWorkBySession(ctx, flagDownloadSessionID)
		if err != nil {
			return err
		}
		if w.VideoPath == "" {
			return clerr.Validation(i18n.T("download.not_exported")).
				WithHint(i18n.T("download.not_exported.hint", flagDownloadSessionID))
		}

		signedURL, err := c.SignedURL(ctx, w.VideoPath)
		if err != nil {
			return err
		}

		rawOut := flagDownloadDest
		if rawOut == "" {
			rawOut = flagDownloadSessionID + ".mp4"
		}
		outPath, err := validate.SafeOutputPath(rawOut)
		if err != nil {
			return clerr.Validation(err.Error()).
				WithHint("--dest must be a relative path inside the current directory")
		}

		if !flagDownloadOverwrite {
			if _, err := os.Stat(outPath); err == nil {
				return clerr.Validationf("file %q already exists; use --overwrite to replace", rawOut)
			}
		}

		format, _ := cmd.Flags().GetString("output")
		stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()

		fmt.Fprintf(stderr, "downloading to %q...\n", rawOut)
		if err := downloadFile(ctx, signedURL, outPath); err != nil {
			return err
		}
		payload := map[string]any{
			"session_id": flagDownloadSessionID,
			"dest":       rawOut,
			"video_path": w.VideoPath,
		}
		switch format {
		case "json":
			return output.NewJSON(stdout).Object(payload)
		case "ndjson":
			payload["type"] = "download.completed"
			return output.NewNDJSON(stdout).Event(payload)
		default:
			// Kept as `output=` rather than `dest=` so shell scripts that
			// already grep this line keep working across the flag rename.
			fmt.Fprintf(stdout, "output=%s\n", rawOut)
			return nil
		}
	},
}

func downloadFile(ctx context.Context, url, dest string) (retErr error) {
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

	// Context-bound so Ctrl-C interrupts a large download instead of
	// leaving the process pinned to an unbounded http.Get. No wall-clock
	// deadline: an MP4 can legitimately take minutes on a slow link, and a
	// guessed timeout would abort valid transfers.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
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
	downloadCmd.Flags().StringVar(&flagDownloadSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	downloadCmd.Flags().StringVar(&flagDownloadDest, "dest", "", "destination file path (default: <session_id>.mp4)")
	downloadCmd.Flags().BoolVar(&flagDownloadOverwrite, "overwrite", false, "overwrite existing output file")
}
