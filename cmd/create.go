package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
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
	"github.com/vibeknow/cli/internal/httpclient"
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
	flagCreateKBID    string
	flagCreatePages   int
	flagCreateImages  string
)

// docIDRe matches a vectoria document identifier supplied via `--from`.
// Two forms are accepted:
//   - Legacy CLI-coined form `doc_<8+ alnum>` (used by older callers).
//   - Vectoria's native UUID form `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`
//     (lowercase hex, what `vk create` itself prints on every run).
//
// Without the UUID branch, a user copying the `doc_id:` line from a prior
// successful run and re-passing it to `--from` would have the CLI treat
// it as a local file path, ending in "stat: no such file or directory".
var docIDRe = regexp.MustCompile(`^(doc_[a-zA-Z0-9]{8,}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

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
		if flagCreatePages != 0 && videoKind != figlens.VideoKindImage2 {
			return clerr.Validation(i18n.T("create.err.pages_needs_image"))
		}
		if flagCreatePages < 0 {
			return clerr.Validation(i18n.T("create.err.pages_invalid", flagCreatePages))
		}
		imageIndexes, err := resolveImageIndexes(flagCreateImages)
		if err != nil {
			return err
		}
		if len(imageIndexes) > 0 {
			// Backend constraints: mandatory images ride the pipeline
			// standard line (default/script/image modes); replica runs an
			// independent graph that rejects them, and the agent engine
			// has no such mechanism.
			if videoKind == figlens.VideoKindReplica {
				return clerr.Validation(i18n.T("create.err.images_not_replica"))
			}
			if engine == figlens.EngineAgent {
				return clerr.Validation(i18n.T("create.err.images_needs_pipeline"))
			}
		}

		ctx := context.Background()

		// Step 1: resolve --from to kb_id + doc_id.
		var kbID, docID string

		switch {
		case docIDRe.MatchString(flagCreateFrom):
			// Direct doc_id — skip upload. --kb-id (printed by `vk doc
			// upload` / a prior create) restores the kb half of the
			// binding; the backend requires the pair together.
			docID = flagCreateFrom
			kbID = strings.TrimSpace(flagCreateKBID)
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
		if len(imageIndexes) > 0 && (kbID == "" || docID == "") {
			// Mandatory-image snapshots are validated against the (user,
			// doc) clips at init time, so the kb/doc pair must be known.
			return clerr.Validation(i18n.T("create.err.images_needs_doc"))
		}

		// Orphan kb cleanup: drop the kb if InitTask never returns OK
		// (task stays nil). os.Exit(5) below skips defers, so an inline
		// call is also needed there.
		var task *figlens.Task
		defer func() {
			if kbID == "" || task != nil {
				return
			}
			cleanupOrphanKB(kbID)
		}()

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
		initParams := figlens.InitTaskParams{Engine: engine, VideoKind: videoKind, SelectedImageIndexes: imageIndexes}
		if (videoKind != "" || len(imageIndexes) > 0) && kbID != "" && docID != "" {
			// Modes with init-time preflights (script_lock quality check,
			// replica PPT check, doc-support check) and mandatory-image
			// snapshots need the kb/doc binding at init, not just on the
			// stream. The pair must travel together — the backend rejects
			// one without the other — so a bare doc_id (no --kb-id) keeps
			// the legacy lean init body, as does the default mode.
			initParams.KnowledgeID = kbID
			initParams.DocID = docID
		}
		task, err = fc.InitTask(ctx, initParams)
		if err != nil {
			// NDJSON consumers expect exactly one terminal task.failed event
			// for every failure regardless of where in the pipeline it
			// happened. Without this synthesis, a pre-stream InitTask
			// failure would leave stdout empty and force consumers to
			// special-case "no terminal event implies it failed before
			// the stream started" — they would have to read stderr to
			// classify the failure, defeating the point of NDJSON output.
			format, _ := cmd.Flags().GetString("output")
			if format == "ndjson" {
				emitPreStreamFailure(cmd.OutOrStdout(), err)
			}

			if errs.HasCode(err, "insufficient_credits") {
				// Mirror the stream-side path's exit code: business failure → 5.
				// os.Exit skips defers — clean up the orphan kb inline first.
				if kbID != "" {
					cleanupOrphanKB(kbID)
				}
				fmt.Fprintln(os.Stderr, i18n.T("credits.insufficient"))
				os.Exit(5)
			}
			var fixable *errs.Object
			if errors.As(err, &fixable) && httpclient.IsUserFixableCode(fixable.Code) {
				// Preflight rejections (script/replica/doc-support): the
				// backend's localized message already lives on the error.
				// Exit 2 via clerr.Validation — these are user-input problems.
				return clerr.Validation(fixable.Message)
			}
			// Retryable codes (rate_limited, internal_error,
			// concurrent_work_limit): exit 4 so agent consumers can branch
			// on "same command will probably succeed if I just wait". The
			// in-stream task.failed path already does this; without this
			// branch, an InitTask-time concurrent_work_limit would exit 1
			// while the same code mid-stream would exit 4 — same condition,
			// different exit code is the exact agent-confusing inconsistency
			// the retryable flag exists to prevent. The deferred orphan-kb
			// cleanup above fires on the return path.
			var o *errs.Object
			if errors.As(err, &o) && httpclient.IsRetryableCode(o.Code) {
				return clerr.Newf("%s", o.Message).WithCode(4)
			}
			return err
		}
		// Past this point, `task != nil` and the backend task owns the kb;
		// the deferred cleanup above will skip on any later error path
		// (task.failed, stream interrupted, --async detach).

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
			TaskID:               task.TaskID,
			SessionID:            task.SessionID,
			Query:                query,
			KnowledgeID:          kbID,
			DocID:                docID,
			VoiceID:              flagCreateVoiceID,
			BGMEnabled:           flagCreateBGM,
			Aspect:               aspect,
			VideoKind:            videoKind,
			PageCount:            flagCreatePages,
			SelectedImageIndexes: imageIndexes,
			Engine:               engine,
		}, func(ev figlens.StreamEvent) {
			switch ev.Type {
			case "node.started", "node.succeeded", "node.warning", "node.failed":
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(ev.NDJSONFields())
				} else {
					switch ev.Type {
					case "node.started":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_started", ev.Node))
					case "node.succeeded":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_succeeded", ev.Node))
					case "node.warning":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_warning", ev.Node, ev.Message))
					case "node.failed":
						fmt.Fprintln(os.Stderr, i18n.T("create.node_failed", ev.Node, ev.Message))
					}
				}
			case "node.progress":
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(ev.NDJSONFields())
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
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(ev.NDJSONFields())
				} else {
					fmt.Fprintln(os.Stderr, i18n.T("create.task_succeeded"))
				}
			case "task.failed":
				switch {
				case httpclient.IsUserFixableCode(ev.Code):
					failExitCode = 2
				case ev.Retryable:
					failExitCode = 4
				default:
					failExitCode = 5
				}
				if isNDJSONCreate {
					_ = output.NewNDJSON(cmd.OutOrStdout()).Event(ev.NDJSONFields())
				} else {
					switch {
					case ev.Code == "insufficient_credits":
						fmt.Fprintln(os.Stderr, i18n.T("credits.insufficient"))
					case httpclient.IsUserFixableCode(ev.Code):
						// Backend's localized preflight message, verbatim.
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
	createCmd.Flags().StringVar(&flagCreateKBID, "kb-id", "", i18n.T("create.flag.kb_id"))
	createCmd.Flags().IntVar(&flagCreatePages, "pages", 0, i18n.T("create.flag.pages"))
	createCmd.Flags().StringVar(&flagCreateImages, "images", "", i18n.T("create.flag.images"))
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
	// Best-effort cleanup if any subsequent step in this function fails.
	// Cleared just before the successful return.
	cleanup := func() {
		// Fresh context with timeout: parent ctx may already be cancelled,
		// and a hung backend must not hold the user hostage.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = vc.DeleteKB(c, kbID)
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

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
	cleanup = nil // ownership transfers to caller from here
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
	cleanup := func() {
		// Fresh context with timeout: parent ctx may already be cancelled,
		// and a hung backend must not hold the user hostage.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = vc.DeleteKB(c, kbID)
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	fmt.Fprintln(os.Stderr, i18n.T("create.uploading_url", url))
	doc, err := vc.UploadURL(ctx, kbID, url)
	if err != nil {
		return "", "", err
	}

	docID, err := pollDocReady(ctx, vc, kbID, doc.ID)
	if err != nil {
		return "", "", err
	}
	cleanup = nil
	return kbID, docID, nil
}

// emitPreStreamFailure writes a synthetic task.failed NDJSON event for
// errors raised before the SSE stream opens (InitTask, future pre-stream
// hooks). The wire shape mirrors the in-stream task.failed event emitted
// by StreamEvent.NDJSONFields so consumers don't have to special-case
// where the failure happened: every CLI exit ≠ 0 in `--output ndjson`
// mode ships exactly one terminal task.failed line on stdout.
//
// Implementation deliberately mirrors NDJSONFields manually instead of
// constructing a fake StreamEvent — the SSE path is the source of truth
// for the stream-side shape, and faking events into it would be a foot
// gun if NDJSONFields gains divergent semantics.
func emitPreStreamFailure(w io.Writer, err error) {
	code := ""
	msg := err.Error()
	var o *errs.Object
	if errors.As(err, &o) {
		code = o.Code
		msg = o.Message
	}
	_ = output.NewNDJSON(w).Event(map[string]any{
		"type":      "task.failed",
		"code":      code,
		"message":   msg,
		"retryable": httpclient.IsRetryableCode(code),
	})
}

// cleanupOrphanKB best-effort deletes a kb the CLI created when the
// backend never claimed it. Errors are swallowed: hygiene, not correctness.
func cleanupOrphanKB(kbID string) {
	vc, err := cliauth.NewVectoriaClient()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = vc.DeleteKB(ctx, kbID)
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
	case "image":
		// User-facing name for the 讲稿生图 line; the wire value carries
		// an internal model codename we deliberately do not surface.
		return figlens.VideoKindImage2, nil
	default:
		return "", clerr.Validation(i18n.T("create.err.mode_invalid", flag))
	}
}

// resolveImageIndexes parses --images ("1,3,5") into mandatory-image
// image_index values, as printed by `vk doc images`. Empty input passes
// through as nil (caller omits the field); duplicates and whitespace are
// tolerated, anything non-positive or non-numeric is a Validation error.
func resolveImageIndexes(flag string) ([]int, error) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return nil, nil
	}
	seen := make(map[int]bool)
	var out []int
	for _, part := range strings.Split(flag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, clerr.Validation(i18n.T("create.err.images_invalid", part))
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, nil
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
	if engine == figlens.EngineAgent && videoKind == figlens.VideoKindImage2 {
		return clerr.Validation(i18n.T("create.err.image_needs_pipeline"))
	}
	return nil
}

