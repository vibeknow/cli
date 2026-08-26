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
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/jobs"
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
	flagCreateTheme   string
	flagCreateLang    string

	flagCreateAvatar    string
	flagCreateAvatarPos string
	flagCreateAvatarPx  float64

	flagCreateScriptLock bool
	flagCreatePreviewDir string
	flagCreateConfirm    string
)

// asyncDetachTimeout bounds how long `--async` waits for the backend's first
// stream event before detaching anyway. Reaching it is not fatal (the run
// request has been delivered) but is reported, since the run was never
// observed to start.
const asyncDetachTimeout = 60 * time.Second

// streamStallNotice is how long the progress stream may stay silent before
// the human path prints a "still generating" reassurance. Long enough that
// ordinary node gaps never trigger it; short enough that the hand-drawn
// line's silent middle section (minutes) reassures more than once.
const streamStallNotice = 60 * time.Second

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
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "create",
	Short: "turn a document, URL, or file into a video",
	Long: `create resolves --from to a document, then generates a video via the figlens pipeline.

--from accepts:
  - a doc_id (e.g. doc_abc12345) — reused directly, and requires --kb-id
    (the backend only accepts a document together with its knowledge base)
  - a URL (http:// or https://) — uploaded to vectoria
  - a local file path — uploaded to vectoria`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Expanded first, so everything below — validation included — sees
		// one flag set and cannot tell a preset value from a typed one.
		// Reported immediately rather than with the rest of the run channel,
		// because a preset that supplied an invalid combination has to be
		// visible in the error that rejects it.
		if err := applyCreatePreset(cmd); err != nil {
			return err
		}

		if flagCreateFrom == "" {
			return clerr.Validation("--from is required").
				WithHint("pass a local file, an http(s) URL, or a doc_id together with --kb-id")
		}

		// --export runs after the preview snapshot, which --async never
		// reaches: it detaches as soon as the run is confirmed. Combining
		// them used to drop --export on the floor and still exit 0, so a
		// caller asking for an MP4 got a preview and no way to tell.
		if flagCreateAsync && flagCreateExport {
			return clerr.Validation(i18n.T("create.err.async_with_export"))
		}

		videoKind, scriptLock, deprecatedModeAlias, err := resolveMode(flagCreateMode, flagCreateScriptLock)
		if err != nil {
			return err
		}
		if deprecatedModeAlias {
			fmt.Fprintln(os.Stderr, i18n.T("create.warn.mode_script_deprecated"))
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
		// 1..20 is the backend's accepted range (Image2MaxPageCount);
		// rejecting locally saves an init round-trip that would 400 anyway.
		if flagCreatePages < 0 || flagCreatePages > 20 {
			return clerr.Validation(i18n.T("create.err.pages_invalid", flagCreatePages))
		}
		// theme and language are read by the v=3 pipeline entry only; the
		// agent handler ignores both, which would silently drop the user's
		// explicit choice — reject the combination like the mode combos.
		if engine == figlens.EngineAgent && flagCreateTheme != "" {
			return clerr.Validation(i18n.T("create.err.theme_needs_pipeline"))
		}
		language, err := resolveLanguage(flagCreateLang)
		if err != nil {
			return err
		}
		if engine == figlens.EngineAgent && language != "" {
			return clerr.Validation(i18n.T("create.err.language_needs_pipeline"))
		}
		avatarRef, err := resolveAvatar(flagCreateAvatar, flagCreateAvatarPos, flagCreateAvatarPx)
		if err != nil {
			return err
		}
		if avatarRef != "" {
			// Both rejections guard against a silent no-op, not a backend
			// error: the agent engine stores the avatar config but has no
			// compositing wired up yet, and the hand-drawn graph has no
			// avatar node at all — either way the backend accepts the
			// request and renders without the presenter the user asked for.
			if engine == figlens.EngineAgent {
				return clerr.Validation(i18n.T("create.err.avatar_needs_pipeline"))
			}
			if videoKind == figlens.VideoKindHandDraw {
				return clerr.Validation(i18n.T("create.err.avatar_not_handdraw"))
			}
		}
		imageIndexes, err := resolveImageIndexes(flagCreateImages)
		if err != nil {
			return err
		}
		if len(imageIndexes) > 0 {
			// Backend constraints: mandatory images ride the pipeline
			// standard/image2/hand-draw graphs (all of which run the
			// knowledge node that consumes them); replica runs an
			// independent graph that rejects them outright, and the agent
			// engine has no such mechanism.
			if videoKind == figlens.VideoKindReplica {
				return clerr.Validation(i18n.T("create.err.images_not_replica"))
			}
			if engine == figlens.EngineAgent {
				return clerr.Validation(i18n.T("create.err.images_needs_pipeline"))
			}
		}

		// Built before anything is uploaded or billed: a --preview-dir that
		// cannot be created is a mistake worth catching while it is still
		// free, not after a render the caller then cannot be shown.
		ch, err := cmdutil.NewRunChannel(cmd, flagCreatePreviewDir)
		if err != nil {
			return clerr.Validation(err.Error())
		}

		ctx := context.Background()

		// Resolved before any upload: a bad voice reference should cost the
		// user nothing, not a knowledgebase and a parsed document.
		voiceID, err := resolveVoiceID(ctx, flagCreateVoiceID)
		if err != nil {
			return err
		}

		// Step 1: resolve --from to kb_id + doc_id.
		var kbID, docID string

		switch {
		case docIDRe.MatchString(flagCreateFrom):
			// Direct doc_id — skip upload. --kb-id (printed by `vk doc
			// upload` / a prior create) restores the kb half of the
			// binding; the backend requires the pair together.
			docID = flagCreateFrom
			kbID = strings.TrimSpace(flagCreateKBID)
			if kbID == "" {
				// The backend rejects a doc_id without its knowledge_id, but
				// only once the stream opens — by which point a task, session
				// and work row exist and the user is reading a raw backend
				// message about arguments they never saw. There is no way to
				// look the pair up client-side (every vectoria document call
				// is scoped by kb), so refuse here with the fix in hand.
				return clerr.Validation(i18n.T("create.err.doc_needs_kb", docID))
			}
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

		if scriptLock && kbID == "" {
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
			// The optimize endpoint keys 原稿锁定 off the video_kind string
			// rather than a boolean (it predates the split and only uses the
			// value to pick a fixed prompt to echo back), so the two axes
			// have to be flattened back into one here. script_lock wins:
			// with the script locked, the prompt is fixed regardless of
			// which line will render it.
			optimizeKind := videoKind
			if scriptLock {
				optimizeKind = figlens.OptimizeVideoKindScriptLock
			}
			optimized, err := fc.FastQueryOptimize(ctx, figlens.OptimizeParams{
				KnowledgeID: kbID,
				DocID:       docID,
				VideoKind:   optimizeKind,
			}, onDelta)
			if streaming {
				fmt.Fprintln(os.Stderr)
			}
			if err != nil {
				// Prompt optimisation is a nice-to-have and degrades to the
				// default query — but not when the reason is authentication.
				// Every later call needs the same credential, so degrading
				// here only buys a confusing "using default" line before the
				// run dies at init anyway. Surface it now, while the message
				// is still the first thing the user reads.
				var ce *clerr.Error
				if errors.As(err, &ce) && ce.Type == clerr.TypeAuth {
					return err
				}
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
		initParams := figlens.InitTaskParams{Engine: engine, VideoKind: videoKind, ScriptLock: scriptLock, SelectedImageIndexes: imageIndexes, PageCount: flagCreatePages}
		if (videoKind != "" || scriptLock || len(imageIndexes) > 0) && kbID != "" && docID != "" {
			// Modes with init-time preflights (script_lock quality check,
			// replica PPT check, doc-support check) and mandatory-image
			// snapshots need the kb/doc binding at init, not just on the
			// stream. The pair must travel together — the backend rejects
			// one without the other — so a bare doc_id (no --kb-id) keeps
			// the legacy lean init body, as does the default mode.
			//
			// scriptLock is its own clause and not folded into videoKind:
			// 原稿锁定 on the standard line leaves videoKind empty, and
			// dropping the binding there would skip the script-quality
			// preflight entirely — the run would only fail after billing.
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

		// Record the run before starting it. Written here rather than after
		// the stream so that a caller killed mid-render — or one whose agent
		// context was discarded — can still find the (task_id, session_id)
		// pair with `vk jobs list` instead of starting a second billed run.
		recordJob(jobs.Record{
			TaskID:    task.TaskID,
			SessionID: task.SessionID,
			WorkID:    task.WorkID,
			Status:    jobs.StatusSubmitted,
			Source:    flagCreateFrom,
			Mode:      videoKind,
			Engine:    engine.String(),
		})

		// Step 4: start generating.
		//
		// StreamChat is the request that actually starts the pipeline —
		// InitTask only reserves the task/session/work rows and returns.
		// It already stamps the work row "generating", which is why a task
		// that never received a stream request looks alive in `vk video
		// list` yet never progresses: nothing is running behind it.
		//
		// So --async cannot skip this call. It differs from the sync path
		// only in *when it stops listening*: it detaches as soon as the
		// backend proves the run is live (first event on the wire) instead
		// of staying for the whole render. The run survives the disconnect
		// — the backend builds its run context from context.Background()
		// and executes it on its own goroutine, so the pipeline is not tied
		// to this HTTP request.
		fmt.Fprintln(os.Stderr, i18n.T("create.generating", task.TaskID, task.SessionID))

		format, _ := cmd.Flags().GetString("output")
		isNDJSONCreate := format == "ndjson"

		// routeEvent sends one stream event to whichever structured channel
		// is active and reports whether it did. When it returns false the
		// caller writes the human line instead, so stderr never carries both
		// renderings of the same fact.
		routeEvent := func(ev figlens.StreamEvent) bool {
			if isNDJSONCreate {
				_ = output.NewNDJSON(cmd.OutOrStdout()).Event(ev.NDJSONFields())
				return true
			}
			if ch.Structured() {
				ch.Emit(ev.NDJSONFields())
				return true
			}
			return false
		}

		var failExitCode int // 0 = not failed; 5 = business; 2 = script_invalid (user-fixable input)
		var successSessionID string
		var sawAnyEvent, taskPaused bool

		// Human progress path only: reassure on long silent stretches. The
		// hand-drawn line's whole middle section emits no process events by
		// design (nothing between script and TTS for minutes), and every
		// mode can go quiet inside a heavy node. Structured consumers get
		// the documented contract ("silence is normal") instead of
		// synthetic events.
		var stall *cmdutil.StallNotifier
		if !isNDJSONCreate && !ch.Structured() {
			stallKey := "create.still_running"
			if videoKind == figlens.VideoKindHandDraw {
				stallKey = "create.still_running_handdraw"
			}
			stall = cmdutil.StartStallNotifier(streamStallNotice, func(elapsed time.Duration) {
				fmt.Fprintln(os.Stderr, i18n.T(stallKey, elapsed))
			})
			defer stall.Stop()
		}

		streamCtx := ctx
		var detach context.CancelFunc
		var detached, detachTimedOut atomic.Bool
		if flagCreateAsync {
			streamCtx, detach = context.WithCancel(ctx)
			defer detach()
			// Backstop: if the backend accepts the request but stays silent,
			// do not hold an "--async" caller indefinitely. The run request
			// has been delivered by then, so the exposure this guards
			// against — the handler not yet having spawned the run — is a
			// sub-second window, not a minute-long one.
			timer := time.AfterFunc(asyncDetachTimeout, func() {
				detachTimedOut.Store(true)
				detach()
			})
			defer timer.Stop()
		}

		err = fc.StreamChat(streamCtx, figlens.StreamParams{
			TaskID:               task.TaskID,
			SessionID:            task.SessionID,
			Query:                query,
			KnowledgeID:          kbID,
			DocID:                docID,
			VoiceID:              voiceID,
			BGMEnabled:           flagCreateBGM,
			Aspect:               aspect,
			VideoKind:            videoKind,
			ScriptLock:           scriptLock,
			PageCount:            flagCreatePages,
			SelectedImageIndexes: imageIndexes,
			Theme:                strings.TrimSpace(flagCreateTheme),
			Language:             language,
			Avatar:               avatarRef,
			AvatarPosition:       strings.TrimSpace(flagCreateAvatarPos),
			AvatarHeightPx:       flagCreateAvatarPx,
			Engine:               engine,
		}, func(ev figlens.StreamEvent) {
			sawAnyEvent = true
			if stall != nil {
				stall.Touch()
			}

			// --async: any event at all proves the backend dispatched the
			// run, which is all this mode promised to wait for. Terminal
			// events still fall through to the handlers below first, so an
			// immediate failure (bad input, no credits) is reported here
			// rather than left for the caller to discover by polling.
			if flagCreateAsync {
				defer func() {
					detached.Store(true)
					detach()
				}()
			}

			switch ev.Type {
			case "node.started", "node.succeeded", "node.warning", "node.failed":
				if !routeEvent(ev) {
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
				if !routeEvent(ev) {
					// [agent] prefix keeps output scannable alongside v=3's [<stage>] lines.
					fmt.Fprintf(os.Stderr, "[agent] %s\n", ev.Message)
				}
			case "task.succeeded":
				successSessionID = ev.SessionID
				if successSessionID == "" {
					successSessionID = task.SessionID
				}
				if !routeEvent(ev) {
					fmt.Fprintln(os.Stderr, i18n.T("create.task_succeeded"))
				}
			case "task.paused":
				taskPaused = true
				if !routeEvent(ev) {
					fmt.Fprintln(os.Stderr, i18n.T("create.task_paused"))
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
				failMsg := ev.Message
				failCode := ev.Code
				updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
					r.Status = jobs.StatusFailed
					r.Error = failMsg
					if r.Error == "" {
						r.Error = failCode
					}
				})
				if !routeEvent(ev) {
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
			// A detach we asked for is not a failure: cancelling the context
			// is how --async stops reading, and it surfaces as a read error.
			if !(flagCreateAsync && errors.Is(err, context.Canceled)) {
				if errs.HasCode(err, "insufficient_credits") {
					updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
						r.Status = jobs.StatusFailed
						r.Error = "insufficient_credits"
					})
					fmt.Fprintln(os.Stderr, i18n.T("credits.insufficient"))
					os.Exit(5)
				}
				// Stream interrupted — exit 6. The run is very likely still
				// going (the backend does not tie it to this connection), so
				// the ledger keeps it as a non-terminal entry that `vk jobs
				// list --active` will surface for reattachment.
				updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
					r.Status = jobs.StatusUnknown
					r.Error = err.Error()
				})
				fmt.Fprintln(os.Stderr, i18n.T("create.stream_interrupted", err))
				os.Exit(6)
			}
		}

		if failExitCode != 0 {
			os.Exit(failExitCode)
		}

		if flagCreateAsync {
			// Report the backstop honestly rather than implying the run is
			// confirmed: the caller should verify before treating it as live.
			if detachTimedOut.Load() && !detached.Load() {
				fmt.Fprintln(os.Stderr, i18n.T("create.async.no_confirmation", int(asyncDetachTimeout.Seconds())))
			}
			// "running" only when an event was actually seen; the backstop
			// path leaves it at "submitted", matching what was observed.
			if detached.Load() {
				updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
					r.Status = jobs.StatusRunning
				})
			}
			// next_actions rides along for the same reason every other JSON
			// payload carries it: it is how a caller decides what to run next
			// without having to have memorised the workflow. --async was the
			// one command that handed back two bare ids and left the caller
			// to infer the follow-up.
			asyncPayload := map[string]any{
				"task_id":    task.TaskID,
				"session_id": task.SessionID,
				"status":     jobs.StatusRunning,
				"next_actions": []map[string]string{{
					"command": fmt.Sprintf("vk video wait %d --session-id %s", task.TaskID, task.SessionID),
					"purpose": "Wait for the generation pipeline to finish",
				}},
			}
			switch format {
			case "json":
				return output.NewJSON(cmd.OutOrStdout()).Object(asyncPayload)
			case "ndjson":
				asyncPayload["event"] = "task.submitted"
				return output.NewNDJSON(cmd.OutOrStdout()).Event(asyncPayload)
			default:
				fmt.Printf("task_id=%d\nsession_id=%s\n", task.TaskID, task.SessionID)
				fmt.Fprintln(os.Stderr, i18n.T("create.async.hint", task.TaskID, task.SessionID))
				return nil
			}
		}

		if taskPaused && successSessionID == "" {
			// Paused is a known non-terminal state, not an unknown one: no
			// probe needed, and re-running create would double-spend. Exit 6
			// (did not reach a terminal state) with the resume path spelled
			// out — resuming is a web-editor action today.
			updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
				r.Status = jobs.StatusPaused
			})
			return clerr.Newf("%s", i18n.T("create.err.task_paused",
				fmt.Sprintf("vk video wait %d --session-id %s", task.TaskID, task.SessionID))).WithCode(6)
		}

		if successSessionID == "" {
			// The stream closed without a terminal event. Returning nil here
			// exited 0 with an empty stdout — no share_url, no error, nothing
			// for the caller to distinguish "finished" from "never ran". That
			// is the same silent success `vk video wait` used to report; the
			// run may well still be going server-side, so exit 6 (state
			// unknown) and point at the ledger entry that can reattach to it.
			updateJob(task.TaskID, task.SessionID, func(r *jobs.Record) {
				r.Status = jobs.StatusUnknown
			})
			// Ask the backend before answering. "The CLI saw nothing" and
			// "nothing happened" are different facts, and only one of them
			// makes it safe to run `vk create` again — which is the very
			// next thing a caller is tempted to do here, at the price of a
			// second render.
			probe := cmdutil.ProbeRun(ctx, fc, task.TaskID, task.SessionID)
			reattach := fmt.Sprintf("vk video wait %d --session-id %s", task.TaskID, task.SessionID)
			if !sawAnyEvent {
				return cmdutil.UnknownStateError(i18n.T("create.err.no_events"), probe, reattach)
			}
			return cmdutil.UnknownStateError(i18n.T("create.err.no_terminal_event"), probe, reattach)
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
		updateJob(task.TaskID, successSessionID, func(r *jobs.Record) {
			r.Status = jobs.StatusSucceeded
			r.WorkID = s.WorkID
			r.Title = s.Title
			r.ShareURL = s.Preview.ShareURL
		})

		// The share_url is a hosted page, which a caller running in a
		// terminal cannot show anyone. With --preview-dir the cover still
		// lands on disk here, so there is something to actually look at
		// before deciding whether the MP4 is worth paying to render.
		cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, fc, w)

		if err := snapshot.Render(stdout, stderr, s, format); err != nil {
			return err
		}

		if !flagCreateExport || !s.Preview.Ready {
			return nil
		}

		// The same spend boundary `vk video export` guards, minted over the
		// same payload — so a caller blocked here can resume with either
		// command and the token still verifies.
		ok, err := cmdutil.Gate(cmd, cmdutil.GateOptions{
			Type:    cmdutil.ExportActionType,
			Payload: cmdutil.ExportActionPayload(successSessionID),
			Prompt:  i18n.T("export.confirm_prompt"),
			Yes:     flagCreateYes,
			Token:   flagCreateConfirm,
			ResumeCommand: func(token string) string {
				return fmt.Sprintf("vk video export %d --session-id %s --confirm %s", task.TaskID, successSessionID, token)
			},
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
		updateJob(task.TaskID, successSessionID, func(r *jobs.Record) {
			r.VideoPath = finalSnap.Export.VideoPath
		})
		cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, fc, w2)
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
	createCmd.Flags().StringVar(&flagCreateVoiceID, "voice", "", "voice from `vk voice list` — either the # or the speech_voice_id")
	createCmd.Flags().StringVar(&flagCreatePrompt, "prompt", "", "custom prompt for video generation (default: auto-generated)")
	createCmd.Flags().BoolVar(&flagCreateAsync, "async", false, "print task_id/session_id and exit without waiting")
	createCmd.Flags().BoolVar(&flagCreateExport, "export", false, "after preview, also render MP4 (extra credits + time)")
	createCmd.Flags().BoolVarP(&flagCreateYes, "yes", "y", false, "skip export confirmation prompt")
	createCmd.Flags().StringVar(&flagCreateMode, "mode", "", i18n.T("create.flag.mode"))
	createCmd.Flags().BoolVar(&flagCreateScriptLock, "script-lock", false, i18n.T("create.flag.script_lock"))
	createCmd.Flags().StringVar(&flagCreateAspect, "aspect", "", i18n.T("create.flag.aspect"))
	createCmd.Flags().BoolVar(&flagCreateBGM, "bgm", false, i18n.T("create.flag.bgm"))
	createCmd.Flags().StringVar(&flagCreateEngine, "engine", "", i18n.T("create.flag.engine"))
	createCmd.Flags().StringVar(&flagCreateKBID, "kb-id", "", i18n.T("create.flag.kb_id"))
	createCmd.Flags().IntVar(&flagCreatePages, "pages", 0, i18n.T("create.flag.pages"))
	createCmd.Flags().StringVar(&flagCreateImages, "images", "", i18n.T("create.flag.images"))
	createCmd.Flags().StringVar(&flagCreateTheme, "theme", "", i18n.T("create.flag.theme"))
	createCmd.Flags().StringVar(&flagCreateLang, "language", "", i18n.T("create.flag.language"))
	createCmd.Flags().StringVar(&flagCreateAvatar, "avatar", "", i18n.T("create.flag.avatar"))
	createCmd.Flags().StringVar(&flagCreateAvatarPos, "avatar-position", "", i18n.T("create.flag.avatar_position"))
	createCmd.Flags().Float64Var(&flagCreateAvatarPx, "avatar-size", 0, i18n.T("create.flag.avatar_size"))
	createCmd.Flags().StringVar(&flagCreatePreviewDir, "preview-dir", "", i18n.T("create.flag.preview_dir"))
	createCmd.Flags().StringVar(&flagCreateConfirm, "confirm", "", "action_id from a previously blocked --export, once the user has agreed to the spend")
	// `auth login` spells "do not block" --no-wait. Same intent here.
	cmdutil.AliasFlags(createCmd, map[string]string{"no-wait": "async"})
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

// numericRe matches a --voice value given as the reference number printed in
// the first column of `vk voice list`, rather than a speech_voice_id.
var numericRe = regexp.MustCompile(`^[0-9]+$`)

// resolveVoiceID turns whatever the user passed to --voice into the value the
// backend's TTS actually keys on, the speech_voice_id.
//
// `vk voice list` prints two identifiers per row and only the long one works;
// passing the short one used to be accepted silently and blow up minutes
// later inside the TTS node with "音色不存在" — after cover and background
// images had already been generated and billed. Rather than making the user
// learn which column is real, a numeric value is looked up and translated
// here, before anything is uploaded or spent.
//
// Non-numeric values pass through untouched: they are already speech_voice_ids,
// and cloned voices do not appear in the template list, so a "not in the list"
// check would reject valid input.
func resolveVoiceID(ctx context.Context, flag string) (string, error) {
	ref := strings.TrimSpace(flag)
	if !voiceRefNeedsLookup(ref) {
		return ref, nil
	}

	_, url, tp, err := cmdutil.Default().Service("vibeknow")
	if err != nil {
		return "", err
	}
	catalog, err := vibeknow.New(url, tp).ListPipelineVoices(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T("create.err.voice_lookup_failed"), err)
	}
	// Flatten includes cloned voices, so `--voice <#>` works for the
	// user's own clones too, not just public templates.
	return mapVoiceRef(ref, catalog.Flatten())
}

// voiceRefNeedsLookup reports whether a --voice value is a `vk voice list`
// reference number that has to be translated, as opposed to a speech_voice_id
// that can go to the backend as-is.
func voiceRefNeedsLookup(ref string) bool {
	return ref != "" && numericRe.MatchString(ref)
}

// mapVoiceRef translates a list reference number into its speech_voice_id.
func mapVoiceRef(ref string, templates []vibeknow.VoiceTemplate) (string, error) {
	for _, t := range templates {
		if strconv.Itoa(t.ID) != ref {
			continue
		}
		if t.SpeechVoiceID == "" {
			// A listed template with no usable id is a backend data
			// problem; failing here beats forwarding an empty voice.
			return "", clerr.Validation(i18n.T("create.err.voice_no_speech_id", ref, t.Name))
		}
		fmt.Fprintln(os.Stderr, i18n.T("create.voice_resolved", ref, t.Name, t.SpeechVoiceID))
		return t.SpeechVoiceID, nil
	}
	return "", clerr.Validation(i18n.T("create.err.voice_unknown_ref", ref))
}

// resolveMode maps --mode and --script-lock onto the two *orthogonal*
// backend parameters they actually drive: video_kind (which pipeline graph
// runs) and script_lock (whether that graph writes a script or uses the
// document verbatim).
//
// They used to be one axis — 原稿锁定 was the video_kind value "script_lock"
// — and `--mode script` still spells it that way. It is now a deprecated
// alias: it resolves to no video_kind (the standard line) plus script_lock,
// which is what the old value meant back when the axes were fused.
//
// Empty --mode passes through as an empty video_kind (caller omits the
// field); an unrecognized value is a Validation error.
func resolveMode(mode string, scriptLockFlag bool) (videoKind string, scriptLock bool, deprecatedAlias bool, err error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return "", scriptLockFlag, false, nil
	case "replica":
		return figlens.VideoKindReplica, scriptLockFlag, false, nil
	case "image":
		// User-facing name for the 讲稿生图 line; the wire value carries
		// an internal model codename we deliberately do not surface.
		return figlens.VideoKindImage2, scriptLockFlag, false, nil
	case "handdraw":
		return figlens.VideoKindHandDraw, scriptLockFlag, false, nil
	case "script":
		return "", true, true, nil
	default:
		return "", false, false, clerr.Validation(i18n.T("create.err.mode_invalid", mode))
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

// createLanguages is the locale set the backend's language allowlist
// accepts for v=3 generation. Anything else silently falls back to the
// deployment default server-side, so the CLI rejects unknowns up front
// instead of letting an explicit choice quietly not happen.
var createLanguages = []string{"zh-CN", "en-US", "es-ES", "fr-FR", "pt-BR", "ja-JP", "ko-KR"}

// resolveLanguage normalizes --language case-insensitively to the backend's
// canonical locale spelling. Empty passes through (deployment default).
func resolveLanguage(flag string) (string, error) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return "", nil
	}
	for _, l := range createLanguages {
		if strings.EqualFold(flag, l) {
			return l, nil
		}
	}
	return "", clerr.Validation(i18n.T("create.err.language_invalid", flag, strings.Join(createLanguages, ", ")))
}

// resolveAvatar validates the --avatar flag trio locally, before any
// upload or init call is spent. Reference shape, position enum, and the
// 120–480 size range are all hard 400s at the backend's stream entry, so
// there is nothing speculative about rejecting them here — and position/
// size without an avatar would be silently meaningless.
//
// Free-drag center coordinates (avatarX/avatarY) are deliberately not
// exposed: they exist for the web editor's drag gesture; corner presets
// plus a size are the whole sensible CLI surface. Empty position/size
// fall back server-side (saved preference → asset default → top-left/240).
func resolveAvatar(ref, position string, heightPx float64) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		if strings.TrimSpace(position) != "" || heightPx != 0 {
			return "", clerr.Validation(i18n.T("create.err.avatar_opts_need_avatar"))
		}
		return "", nil
	}
	if !strings.HasPrefix(ref, figlens.AvatarRefSystemPrefix) && !strings.HasPrefix(ref, figlens.AvatarRefUserPrefix) {
		return "", clerr.Validation(i18n.T("create.err.avatar_ref_invalid", ref))
	}
	switch strings.TrimSpace(position) {
	case "", "top-left", "top-right", "bottom-left", "bottom-right":
	default:
		return "", clerr.Validation(i18n.T("create.err.avatar_position_invalid", position))
	}
	if heightPx != 0 && (heightPx < figlens.AvatarMinHeightPx || heightPx > figlens.AvatarMaxHeightPx) {
		return "", clerr.Validation(i18n.T("create.err.avatar_size_invalid", heightPx))
	}
	return ref, nil
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
//
// All three rejected modes are pipeline-only for the same structural reason:
// each runs a dedicated graph selected by video_kind, and the agent engine
// dispatches on none of them — it would accept the request and quietly run
// its ordinary line instead.
func validateEngineModeCombo(engine figlens.Engine, videoKind string) error {
	if engine != figlens.EngineAgent {
		return nil
	}
	switch videoKind {
	case figlens.VideoKindReplica:
		return clerr.Validation(i18n.T("create.err.replica_needs_pipeline"))
	case figlens.VideoKindImage2:
		return clerr.Validation(i18n.T("create.err.image_needs_pipeline"))
	case figlens.VideoKindHandDraw:
		return clerr.Validation(i18n.T("create.err.handdraw_needs_pipeline"))
	}
	return nil
}
