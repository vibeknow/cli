package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/client/figlens"
	"github.com/shiliu-ai/vibeknow-cli/client/vectoria"
	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
	"github.com/shiliu-ai/vibeknow-cli/internal/errs"
)

var (
	flagCreateFrom    string
	flagCreateVoiceID string
	flagCreatePrompt  string
	flagCreateAsync   bool
)

var docIDRe = regexp.MustCompile(`^doc_[a-zA-Z0-9]{8,}$`)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "turn a document, URL, or file into a video",
	Long: `create resolves --from to a document, then generates a video via the figlens pipeline.

--from accepts:
  - a doc_id (e.g. doc_abc12345) — used directly
  - a URL (http:// or https://) — uploaded to vectoria
  - a local file path — uploaded to vectoria`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagCreateFrom == "" {
			return fmt.Errorf("--from is required")
		}

		ctx := context.Background()

		// Step 1: resolve --from to kb_id + doc_id.
		var kbID, docID string

		switch {
		case docIDRe.MatchString(flagCreateFrom):
			// Direct doc_id — skip upload.
			docID = flagCreateFrom
			fmt.Fprintf(os.Stderr, "using doc_id: %s\n", docID)

		case strings.HasPrefix(flagCreateFrom, "http://") || strings.HasPrefix(flagCreateFrom, "https://"):
			// URL upload.
			var err error
			kbID, docID, err = uploadURL(ctx, flagCreateFrom)
			if err != nil {
				return err
			}

		default:
			// Local file.
			var err error
			kbID, docID, err = uploadFile(ctx, flagCreateFrom)
			if err != nil {
				return err
			}
		}

		// Step 2: optimize prompt (skip if user provided --prompt).
		fc, err := newCreateFiglensClient()
		if err != nil {
			return err
		}

		query := flagCreatePrompt
		if query == "" && kbID != "" && docID != "" {
			fmt.Fprintf(os.Stderr, "optimising prompt...\n")
			optimized, err := fc.FastQueryOptimize(ctx, figlens.OptimizeParams{
				KnowledgeID: kbID,
				DocID:       docID,
			}, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "prompt optimisation failed, using default: %s\n", err)
				query = "根据文档内容生成视频"
			} else {
				query = optimized
				fmt.Fprintf(os.Stderr, "prompt: %s\n", query)
			}
		} else if query == "" {
			query = "根据文档内容生成视频"
		}

		// Step 3: init figlens task.
		fmt.Fprintf(os.Stderr, "initialising task...\n")
		task, err := fc.InitTask(ctx)
		if err != nil {
			if errs.HasCode(err, "insufficient_credits") {
				return fmt.Errorf("积分不足，请充值后再试")
			}
			return err
		}

		// Step 4: async or sync.
		if flagCreateAsync {
			fmt.Printf("task_id=%d\nsession_id=%s\n", task.TaskID, task.SessionID)
			fmt.Fprintf(os.Stderr, "hint: run `vibeknow video wait %d --session-id %s` to track progress\n",
				task.TaskID, task.SessionID)
			return nil
		}

		// Step 5: stream with progress.
		fmt.Fprintf(os.Stderr, "generating video (task_id=%d session_id=%s)...\n", task.TaskID, task.SessionID)

		var taskFailed bool
		var successSessionID string

		err = fc.StreamChat(ctx, figlens.StreamParams{
			TaskID:      task.TaskID,
			SessionID:   task.SessionID,
			Query:       query,
			KnowledgeID: kbID,
			DocID:       docID,
			VoiceID:     flagCreateVoiceID,
		}, func(ev figlens.StreamEvent) {
			switch ev.Type {
			case "node.started":
				fmt.Fprintf(os.Stderr, "[%s] started\n", ev.Node)
			case "node.succeeded":
				fmt.Fprintf(os.Stderr, "[%s] done\n", ev.Node)
			case "node.failed":
				fmt.Fprintf(os.Stderr, "[%s] failed: %s\n", ev.Node, ev.Message)
			case "task.succeeded":
				successSessionID = ev.SessionID
				if successSessionID == "" {
					successSessionID = task.SessionID
				}
				fmt.Fprintf(os.Stderr, "task succeeded\n")
			case "task.failed":
				taskFailed = true
				if strings.Contains(ev.Message, "insufficient_credits") {
					fmt.Fprintf(os.Stderr, "积分不足，请充值后再试\n")
				} else {
					fmt.Fprintf(os.Stderr, "task failed: %s\n", ev.Message)
				}
			}
		})
		if err != nil {
			if errs.HasCode(err, "insufficient_credits") {
				fmt.Fprintf(os.Stderr, "积分不足，请充值后再试\n")
				os.Exit(5)
			}
			// Stream interrupted — exit 6.
			fmt.Fprintf(os.Stderr, "stream interrupted: %s\n", err)
			os.Exit(6)
		}

		if taskFailed {
			os.Exit(5)
		}

		// Step 5: fetch work detail.
		if successSessionID != "" {
			w, err := fc.GetWorkBySession(ctx, successSessionID)
			if err != nil {
				return err
			}
			fmt.Printf("task_id=%d\n", task.TaskID)
			fmt.Printf("session_id=%s\n", successSessionID)
			fmt.Printf("work_id=%d\n", w.ID)
			fmt.Printf("title=%s\n", w.Title)
			if w.VideoPath != "" {
				fmt.Printf("video_path=%s\n", w.VideoPath)
			}
			if w.Duration > 0 {
				fmt.Printf("duration=%d\n", w.Duration)
			}
		}

		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&flagCreateFrom, "from", "", "doc_id, URL, or local file path (required)")
	createCmd.Flags().StringVar(&flagCreateVoiceID, "voice", "", "voice template ID")
	createCmd.Flags().StringVar(&flagCreatePrompt, "prompt", "", "custom prompt for video generation (default: auto-generated)")
	createCmd.Flags().BoolVar(&flagCreateAsync, "async", false, "print task_id/session_id and exit without waiting")
}

// uploadFile uploads a local file to vectoria and returns kb_id + doc_id.
func uploadFile(ctx context.Context, filePath string) (string, string, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", "", fmt.Errorf("stat %q: %w", filePath, err)
	}
	if !fi.Mode().IsRegular() {
		return "", "", fmt.Errorf("%q is not a regular file", filePath)
	}

	vc, err := newVectoriaClient()
	if err != nil {
		return "", "", err
	}

	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	fmt.Fprintf(os.Stderr, "creating knowledge base %q...\n", kbName)
	kbID, err := vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("open %q: %w", filePath, err)
	}
	defer f.Close()

	fmt.Fprintf(os.Stderr, "uploading %q...\n", fi.Name())
	doc, err := vc.UploadDoc(ctx, kbID, fi.Name(), f)
	if err != nil {
		return "", "", err
	}

	docID, err := pollDocReady(ctx, vc, kbID, doc.ID)
	if err != nil {
		return "", "", err
	}
	return kbID, docID, nil
}

// uploadURL uploads a URL to vectoria and returns kb_id + doc_id.
func uploadURL(ctx context.Context, url string) (string, string, error) {
	vc, err := newVectoriaClient()
	if err != nil {
		return "", "", err
	}

	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	fmt.Fprintf(os.Stderr, "creating knowledge base %q...\n", kbName)
	kbID, err := vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", err
	}

	fmt.Fprintf(os.Stderr, "uploading URL %q...\n", url)
	doc, err := vc.UploadURL(ctx, kbID, url)
	if err != nil {
		return "", "", err
	}

	docID, err := pollDocReady(ctx, vc, kbID, doc.ID)
	if err != nil {
		return "", "", err
	}
	return kbID, docID, nil
}

// pollDocReady polls until the document is completed or fails.
func pollDocReady(ctx context.Context, vc *vectoria.Client, kbID, docID string) (string, error) {
	fmt.Fprintf(os.Stderr, "doc_id: %s — polling for completion...\n", docID)
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for document processing (10m)")
		}
		d, err := vc.GetDocStatus(ctx, kbID, docID)
		if err != nil {
			return "", err
		}
		switch d.Status {
		case "completed":
			fmt.Fprintf(os.Stderr, "document ready\n")
			return d.ID, nil
		case "failed", "error":
			return "", fmt.Errorf("document processing failed: %s", d.Error)
		default:
			fmt.Fprintf(os.Stderr, "document status: %s\n", d.Status)
			time.Sleep(2 * time.Second)
		}
	}
}

func newVectoriaClient() (*vectoria.Client, error) {
	apiKey := os.Getenv("VECTORIA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("VECTORIA_API_KEY env var is required")
	}
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	url, err := endpoints.Resolve(p, "vectoria")
	if err != nil {
		return nil, err
	}
	return vectoria.New(url, apiKey), nil
}

func newCreateFiglensClient() (*figlens.Client, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	tok, _, err := cliauth.ResolverFor(p).Resolve()
	if err != nil {
		return nil, fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
	}
	url, err := endpoints.Resolve(p, "figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, createStaticToken(tok)), nil
}

type createStaticToken string

func (s createStaticToken) Token(_ context.Context) (string, error) { return string(s), nil }
