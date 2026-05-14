package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/exportpoll"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

var (
	flagCreateFrom    string
	flagCreateVoiceID string
	flagCreatePrompt  string
	flagCreateAsync   bool
	flagCreateExport  bool
	flagCreateYes     bool
	flagCreateMode    string
	flagCreateAspect  string
	flagCreateBGM     bool
	flagCreateEngine  string
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

		videoKind, err := resolveVideoKind(flagCreateMode)
		if err != nil {
			return err
		}
		aspect, err := resolveAspect(flagCreateAspect)
		if err != nil {
			return err
		}
		engine, err := resolveEngine(flagCreateEngine)
		if err != nil {
			return err
		}
		if err := validateEngineModeCombo(engine, videoKind); err != nil {
			return err
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

		if videoKind == figlens.VideoKindScriptLock && kbID == "" {
			return clerr.Validation(i18n.T("create.err.script_needs_doc"))
		}

		// Step 2: optimize prompt (skip if user provided --prompt).
		_, url, tp, err := cmdutil.Default().Service("figlens")
		if err != nil {
			return err
		}
		fc := figlens.New(url, tp)

		query := flagCreatePrompt
		if query == "" && kbID != "" && docID != "" {
			streaming := term.IsTerminal(int(os.Stderr.Fd()))
			var onDelta func(string)
			if streaming {
				fmt.Fprint(os.Stderr, i18n.T("create.prompt_prefix"))
				onDelta = func(s string) { fmt.Fprint(os.Stderr, s) }
			} else {
				fmt.Fprintln(os.Stderr, i18n.T("create.optimising_prompt"))
			}
			optimized, err := fc.FastQueryOptimize(ctx, figlens.OptimizeParams{
				KnowledgeID: kbID,
				DocID:       docID,
				VideoKind:   videoKind,
			}, onDelta)
			if streaming {
				fmt.Fprintln(os.Stderr)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("create.prompt_fallback", err))
				query = i18n.T("create.default_query")
			} else {
				query = optimized
				if !streaming {
					fmt.Fprintf(os.Stderr, "%s%s\n", i18n.T("create.prompt_prefix"), query)
				}
			}
		} else if query == "" {
			query = i18n.T("create.default_query")
		}

		// Step 3: init figlens task.
		fmt.Fprintln(os.Stderr, i18n.T("create.init_task"))
		initParams := figlens.InitTaskParams{Engine: engine, VideoKind: videoKind}
		if videoKind == figlens.VideoKindScriptLock {
			initParams.KnowledgeID = kbID
			initParams.DocID = docID
		}
		task, err := fc.InitTask(ctx, initParams)
		if err != nil {
			if errs.HasCode(err, "insufficient_credits") {
				return fmt.Errorf("%s", i18n.T("credits.insufficient"))
			}
			if errs.HasCode(err, "script_invalid") {
				// Backend's localized message already lives on the error.
				// Exit 2 via clerr.Validation — this is a user-input problem.
				var o *errs.Object
				if errors.As(err, &o) {
					return clerr.Validation(o.Message)
				}
				return clerr.Validation(err.Error())
			}
			return err
		}

		// Step 4: async or sync.
		if flagCreateAsync {
			fmt.Printf("task_id=%d\nsession_id=%s\n", task.TaskID, task.SessionID)
			fmt.Fprintln(os.Stderr, i18n.T("create.async.hint", task.TaskID, task.SessionID))
			return nil
		}

		// Step 5: stream with progress.
		fmt.Fprintln(os.Stderr, i18n.T("create.generating", task.TaskID, task.SessionID))

		format, _ := cmd.Flags().GetString("output")
		isNDJSONCreate := format == "ndjson"

		var failExitCode int // 0 = not failed; 5 = business; 2 = script_invalid (user-fixable input)
		var successSessionID string

		err = fc.StreamChat(ctx, figlens.StreamParams{
			TaskID:      task.TaskID,
			SessionID:   task.SessionID,
			Query:       query,
			KnowledgeID: kbID,
			DocID:       docID,
			VoiceID:     flagCreateVoiceID,
			BGMEnabled:  flagCreateBGM,
			Aspect:      aspect,
			VideoKind:   videoKind,
			Engine:      engine,
		}, func(ev figlens.StreamEvent) {
			switch ev.Type {
			case "node.started", "node.succeeded", "node.failed":
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(map[string]any{
						"type": ev.Type, "stage": ev.Stage, "node": ev.Node, "message": ev.Message,
					})
				} else {
					switch ev.Type {
					case "node.started":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_started", ev.Node))
					case "node.succeeded":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_succeeded", ev.Node))
					case "node.failed":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_failed", ev.Node, ev.Message))
					}
				}
			case "node.progress":
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(map[string]any{
						"type":    "node.progress",
						"status":  ev.Status,
						"message": ev.Message,
					})
				} else {
					// [agent] prefix keeps output scannable alongside v=3's [<stage>] lines.
					fmt.Fprintf(os.Stderr, "[agent] %s\n", ev.Message)
				}
			case "task.succeeded":
				successSessionID = ev.SessionID
				if successSessionID == "" {
					successSessionID = task.SessionID
				}
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(map[string]any{
						"type": "task.succeeded", "session_id": ev.SessionID,
					})
				} else {
					fmt.Fprintln(os.Stderr, i18n.T("create.task_succeeded"))
				}
			case "task.failed":
				failExitCode = 5
				if ev.Code == "script_invalid" {
					failExitCode = 2
				}
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(map[string]any{
						"type":    "task.failed",
						"code":    ev.Code,
						"message": ev.Message,
					})
				} else {
					switch ev.Code {
					case "insufficient_credits":
						fmt.Fprintln(os.Stderr, i18n.T("credits.insufficient"))
					case "script_invalid":
						fmt.Fprintln(os.Stderr, ev.Message)
					default:
						fmt.Fprintln(os.Stderr, i18n.T("create.task_failed", ev.Message))
					}
				}
			}
		})
		if err != nil {
			if errs.HasCode(err, "insufficient_credits") {
				fmt.Fprintln(os.Stderr, i18n.T("credits.insufficient"))
				os.Exit(5)
			}
			// Stream interrupted — exit 6.
			fmt.Fprintln(os.Stderr, i18n.T("create.stream_interrupted", err))
			os.Exit(6)
		}

		if failExitCode != 0 {
			os.Exit(failExitCode)
		}

		if successSessionID == "" {
			return nil
		}

		stdout := cmd.OutOrStdout()
		stderr := cmd.ErrOrStderr()
		shareBase := cmdutil.ShareBaseURL()

		w, err := fc.GetWorkBySession(ctx, successSessionID)
		if err != nil {
			return err
		}
		s := snapshot.Build(snapshot.BuildInput{
			TaskID:    task.TaskID,
			SessionID: successSessionID,
			Work:      w,
			ShareBase: shareBase,
		})
		if err := snapshot.Render(stdout, stderr, s, format); err != nil {
			return err
		}

		if !flagCreateExport || !s.Preview.Ready {
			return nil
		}

		ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
			Prompt: i18n.T("export.confirm_prompt"),
			Yes:    flagCreateYes,
		})
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stderr, i18n.T("export.cancelled"))
			return nil
		}

		expID, err := fc.ExportVideo(ctx, successSessionID)
		if err != nil {
			fmt.Fprintln(stderr, i18n.T("export.failed", err.Error()))
			os.Exit(7)
		}
		fmt.Fprintln(stderr, i18n.T("export.submitted", expID))

		progressTTY := term.IsTerminal(int(os.Stderr.Fd()))
		result, perr := exportpoll.PollExport(ctx, fc, expID, exportpoll.DefaultTimeout(), 0, func(ev exportpoll.Event) {
			if ev.Status != snapshot.StatusRunning || !progressTTY {
				return
			}
			if ev.ProgressMsg != "" {
				fmt.Fprintf(stderr, "\r%s", i18n.T("export.progress", ev.Progress, ev.ProgressMsg))
			} else {
				fmt.Fprintf(stderr, "\r%s", i18n.T("export.progress_simple", ev.Progress))
			}
		})
		if perr != nil {
			fmt.Fprintln(stderr)
			fmt.Fprintln(stderr, i18n.T("export.failed", perr.Error()))
			os.Exit(7)
		}

		w2, err := fc.GetWorkBySession(ctx, successSessionID)
		if err != nil {
			return err
		}
		finalSnap := snapshot.Build(snapshot.BuildInput{
			TaskID:       task.TaskID,
			SessionID:    successSessionID,
			Work:         w2,
			Export:       result,
			ExportTaskID: expID,
			ShareBase:    shareBase,
		})
		if format == "text" || format == "" {
			fmt.Fprintln(stderr)
		}
		if err := snapshot.Render(stdout, stderr, finalSnap, format); err != nil {
			return err
		}
		if finalSnap.Export.Status == snapshot.StatusFailed {
			os.Exit(7)
		}
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&flagCreateFrom, "from", "", "doc_id, URL, or local file path (required)")
	createCmd.Flags().StringVar(&flagCreateVoiceID, "voice", "", "voice template ID")
	createCmd.Flags().StringVar(&flagCreatePrompt, "prompt", "", "custom prompt for video generation (default: auto-generated)")
	createCmd.Flags().BoolVar(&flagCreateAsync, "async", false, "print task_id/session_id and exit without waiting")
	createCmd.Flags().BoolVar(&flagCreateExport, "export", false, "after preview, also render MP4 (extra credits + time)")
	createCmd.Flags().BoolVarP(&flagCreateYes, "yes", "y", false, "skip export confirmation prompt")
	createCmd.Flags().StringVar(&flagCreateMode, "mode", "", i18n.T("create.flag.mode"))
	createCmd.Flags().StringVar(&flagCreateAspect, "aspect", "", i18n.T("create.flag.aspect"))
	createCmd.Flags().BoolVar(&flagCreateBGM, "bgm", false, i18n.T("create.flag.bgm"))
	createCmd.Flags().StringVar(&flagCreateEngine, "engine", "", i18n.T("create.flag.engine"))
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

	vc, err := cliauth.NewVectoriaClient()
	if err != nil {
		return "", "", err
	}

	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	fmt.Fprintln(os.Stderr, i18n.T("create.creating_kb", kbName))
	kbID, err := vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("open %q: %w", filePath, err)
	}
	defer f.Close()

	fmt.Fprintln(os.Stderr, i18n.T("create.uploading_file", fi.Name()))
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
	vc, err := cliauth.NewVectoriaClient()
	if err != nil {
		return "", "", err
	}

	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	fmt.Fprintln(os.Stderr, i18n.T("create.creating_kb", kbName))
	kbID, err := vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", err
	}

	fmt.Fprintln(os.Stderr, i18n.T("create.uploading_url", url))
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
	fmt.Fprintln(os.Stderr, i18n.T("create.doc_polling", docID))
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s", i18n.T("create.doc_timeout"))
		}
		d, err := vc.GetDocStatus(ctx, kbID, docID)
		if err != nil {
			return "", err
		}
		switch d.Status {
		case "completed":
			fmt.Fprintln(os.Stderr, i18n.T("create.doc_ready"))
			return d.ID, nil
		case "failed", "error":
			return "", fmt.Errorf("%s", i18n.T("create.doc_failed", d.Error))
		default:
			fmt.Fprintln(os.Stderr, i18n.T("create.doc_status", d.Status))
			time.Sleep(2 * time.Second)
		}
	}
}

// resolveVideoKind maps the --mode flag to the backend video_kind wire value.
// Empty passes through (caller omits the field); unrecognized → Validation error.
func resolveVideoKind(flag string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "":
		return "", nil
	case "replica":
		return figlens.VideoKindReplica, nil
	case "script":
		return figlens.VideoKindScriptLock, nil
	default:
		return "", clerr.Validation(i18n.T("create.err.mode_invalid", flag))
	}
}

// resolveAspect normalizes --aspect (canonical words + 16:9 / 9:16 aliases)
// to the backend wire value.
func resolveAspect(flag string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "":
		return "", nil
	case "horizontal", "16:9":
		return "horizontal", nil
	case "vertical", "9:16":
		return "vertical", nil
	default:
		return "", clerr.Validation(i18n.T("create.err.aspect_invalid", flag))
	}
}

// resolveEngine maps the --engine flag to a figlens.Engine value.
// Empty input passes through as EnginePipeline (the zero value)
// so the default invocation is byte-identical to 0.4.2 on the wire.
func resolveEngine(flag string) (figlens.Engine, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "", "pipeline":
		return figlens.EnginePipeline, nil
	case "agent":
		return figlens.EngineAgent, nil
	default:
		return figlens.EnginePipeline, clerr.Validation(i18n.T("create.err.engine_invalid", flag))
	}
}

// validateEngineModeCombo rejects engine+mode combinations the backend doesn't support.
func validateEngineModeCombo(engine figlens.Engine, videoKind string) error {
	if engine == figlens.EngineAgent && videoKind == figlens.VideoKindReplica {
		return clerr.Validation(i18n.T("create.err.replica_needs_pipeline"))
	}
	return nil
}

