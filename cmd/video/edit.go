package video

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagEditSessionID  string
	flagEditScene      int
	flagEditScript     string
	flagEditScriptOnly bool
	flagEditYes        bool
	flagEditConfirm    string
	flagEditPreviewDir string
)

var editCmd = &cobra.Command{
	Use:   "edit [task_id]",
	Short: "rewrite one shot's narration and regenerate it (bills)",
	Args:  cobra.MaximumNArgs(1),
	Long: `Replace what one shot says, and regenerate that shot to match.

This is the first content edit the CLI has: until now the only way to change
a word of narration was a fresh ` + "`create`" + ` at full price, discarding
everything else about the video that was already right.

One shot per call. ` + "`--scene`" + ` takes the number
` + "`vk video script`" + ` prints, counting from 1.

**This bills.** Everything chargeable on this path is downstream of the
narration actually changing, so the cost is knowable before the request goes
out — which is why the confirmation names it. Without a terminal the
decision is handed back as exit 8 rather than made on your behalf; see
` + "`vk video export`" + ` for the same gate.

  --script-only   regenerate the voice-over, timeline and presenter only.
                  Layout, cards and background image stay as they are.
                  Cheaper, and right when the new text is about as long as
                  the old. Wrong when it is much longer: nothing re-flows,
                  so the text can overrun the card it sits in.
  (default)       regenerate the whole shot, layout and background image
                  included. Costs more and cannot overflow.

**The rendered MP4 is not updated and not withdrawn.** The backend leaves it
in place, so ` + "`vk video download`" + ` keeps returning the file made from
the *old* narration until the next ` + "`vk video export`" + `. The preview
and share link do reflect the edit immediately.

**There is no undo.** The backend keeps a rollback snapshot to recover from
its own mid-run failures, but exposes no way to ask for the previous version
back. Read the shot with ` + "`vk video script`" + ` before changing it if
the current wording is worth keeping.`,
	Example: `  vk video script 42
  vk video edit 42 --scene 3 --script "换一种说法，更短一点。"
  vk video edit 42 --scene 3 --script "..." --script-only
  vk video edit 42 --scene 3 --script "..." --confirm act_9f2c...`,
	RunE: runEdit,
}

func runEdit(cmd *cobra.Command, args []string) error {
	if !cmd.Flags().Changed("scene") {
		return clerr.Validation("--scene is required").
			WithHint("run `vk video script` to see the shots and their numbers")
	}
	if !cmd.Flags().Changed("script") {
		return clerr.Validation("--script is required: there is nothing else this command changes").
			WithHint("to change music, subtitles or the title instead, use `vk video set`")
	}
	next := strings.TrimSpace(flagEditScript)
	if next == "" {
		return clerr.Validation("--script cannot be empty").
			WithHint("a shot must say something; there is no command that removes narration from a shot")
	}

	taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagEditSessionID)
	if err != nil {
		return err
	}
	c, err := newFiglensClient()
	if err != nil {
		return err
	}
	ch, err := cmdutil.NewRunChannel(cmd, flagEditPreviewDir)
	if err != nil {
		return clerr.Validation(err.Error())
	}

	work, err := c.GetWorkBySession(cmd.Context(), sessionID)
	if err != nil {
		return err
	}
	scenes, err := c.GetWorkScenes(cmd.Context(), work.ID)
	if err != nil {
		return err
	}
	if len(scenes) == 0 {
		return clerr.New("this work has no shots recorded yet").
			WithCode(6).
			WithHint("a run that is still generating has nothing to edit; check `vk video status` first")
	}
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].SceneIndex < scenes[j].SceneIndex })

	// The request array is positional — entry i is scene i+1 — so a gap or a
	// repeat in the numbering would silently point the edit at the wrong
	// shot. The backend reindexes to 1..N after every delete, so this should
	// hold; refusing beats editing something the caller did not name.
	for i, s := range scenes {
		if s.SceneIndex != i+1 {
			return clerr.Newf("this work's shots are numbered %s, which this command cannot address safely", sceneRange(scenes)).
				WithCode(1).
				WithHint("report this: shot numbering is expected to run 1..N with no gaps")
		}
	}

	target, ok := sceneByIndex(scenes, flagEditScene)
	if !ok {
		return clerr.Validationf("this work has no shot %d", flagEditScene).
			WithHintf("it has %d shots, numbered %s; run `vk video script` to see them", len(scenes), sceneRange(scenes))
	}
	prev := strings.TrimSpace(target.ScriptText)
	if prev == next {
		// Refused locally and for free. The backend would accept this,
		// notice nothing changed, bill nothing and return success — leaving
		// a caller that made a typo in its own diff believing the edit
		// landed.
		return clerr.Validationf("shot %d already says exactly this", flagEditScene).
			WithHint("nothing would change; compare against `vk video script` before editing")
	}

	ok, err = cmdutil.Gate(cmd, cmdutil.GateOptions{
		Type:    cmdutil.SceneEditActionType,
		Payload: cmdutil.SceneEditActionPayload(sessionID, flagEditScene, prev, next, flagEditScriptOnly),
		Prompt:  editPrompt(flagEditScene, flagEditScriptOnly),
		Yes:     flagEditYes,
		Token:   flagEditConfirm,
		ResumeCommand: func(token string) string {
			return editResumeCommand(taskID, sessionID, flagEditScene, flagEditScript, flagEditScriptOnly, token)
		},
	})
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("edit.cancelled"))
		return nil
	}

	params := figlens.SceneEditParams{
		SessionID:  sessionID,
		Scenes:     make([]figlens.SceneEditSceneParams, len(scenes)),
		ScriptOnly: flagEditScriptOnly,
	}
	params.Scenes[flagEditScene-1] = figlens.SceneEditSceneParams{
		Edit:       figlens.SceneEditNormal,
		ScriptText: &next,
	}

	var previewURL string
	streamErr := c.StreamSceneEdit(cmd.Context(), params, func(ev figlens.SceneEditEvent) {
		if ev.PreviewURL != "" {
			previewURL = ev.PreviewURL
		}
		emitEditEvent(cmd, ch, flagEditScene, ev)
	})
	if streamErr != nil {
		return streamErr
	}

	// Cosmetic but not pointless: the ledger's note of a downloadable file
	// is now wrong, and `vk jobs` showing a path for a video that no longer
	// matches its script is the same lie the payload below exists to avoid.
	staleExport := strings.TrimSpace(work.VideoPath) != ""
	if staleExport {
		noteJob(taskID, sessionID, func(r *jobs.Record) { r.VideoPath = "" })
	}
	if ch.Previews != nil {
		if w, err := c.GetWorkBySession(cmd.Context(), sessionID); err == nil {
			cmdutil.DeliverWorkArtifacts(cmd.Context(), ch.Previews, c, w)
		}
	}

	return renderEditResult(cmd, editResult{
		TaskID:      taskID,
		SessionID:   sessionID,
		WorkID:      work.ID,
		SceneIndex:  flagEditScene,
		ScriptOnly:  flagEditScriptOnly,
		From:        prev,
		To:          next,
		PreviewURL:  previewURL,
		StaleExport: staleExport,
	})
}

type editResult struct {
	TaskID      int64
	SessionID   string
	WorkID      int64
	SceneIndex  int
	ScriptOnly  bool
	From        string
	To          string
	PreviewURL  string
	StaleExport bool
}

func renderEditResult(cmd *cobra.Command, r editResult) error {
	format, _ := cmd.Flags().GetString("output")
	if format == output.FormatJSON || format == output.FormatNDJSON {
		payload := map[string]any{
			"task_id":     r.TaskID,
			"session_id":  r.SessionID,
			"work_id":     r.WorkID,
			"scene_index": r.SceneIndex,
			"script_only": r.ScriptOnly,
			"script":      map[string]any{"from": r.From, "to": r.To},
			// Named for what a caller has to decide, not for what the
			// backend did: the file still exists and still downloads, it
			// just no longer matches the work.
			"export_stale": r.StaleExport,
		}
		if r.PreviewURL != "" {
			payload["preview_url"] = r.PreviewURL
		}
		if r.StaleExport {
			payload["next_actions"] = []map[string]string{{
				"command": fmt.Sprintf("vk video export %d --session-id %s", r.TaskID, r.SessionID),
				"purpose": "Re-render the MP4 with the new narration (bills; asks for confirmation first)",
			}}
		}
		return output.NewJSON(cmd.OutOrStdout()).Object(payload)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "scene_index=%d\n", r.SceneIndex)
	if r.PreviewURL != "" {
		fmt.Fprintf(out, "preview_url=%s\n", r.PreviewURL)
	}
	if r.StaleExport {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("edit.export_stale", r.TaskID, r.SessionID))
	}
	return nil
}

// emitEditEvent writes one stream observation to whichever channel the
// caller is reading, never to both.
func emitEditEvent(cmd *cobra.Command, ch *cmdutil.RunChannel, sceneIndex int, ev figlens.SceneEditEvent) {
	if ch.Structured() {
		fields := map[string]any{"type": ev.Type, "scene_index": sceneIndex}
		if ev.Message != "" {
			fields["message"] = ev.Message
		}
		if ev.Status != "" {
			fields["status"] = ev.Status
		}
		if ev.PreviewURL != "" {
			fields["preview_url"] = ev.PreviewURL
		}
		ch.Emit(fields)
		return
	}
	stderr := cmd.ErrOrStderr()
	switch ev.Type {
	case "edit.succeeded":
		fmt.Fprintln(stderr, i18n.T("edit.succeeded", sceneIndex))
	default:
		if ev.Message != "" {
			fmt.Fprintln(stderr, ev.Message)
		}
	}
}

func sceneByIndex(scenes []figlens.WorkScene, idx int) (figlens.WorkScene, bool) {
	for _, s := range scenes {
		if s.SceneIndex == idx {
			return s, true
		}
	}
	return figlens.WorkScene{}, false
}

func sceneRange(scenes []figlens.WorkScene) string {
	if len(scenes) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%d–%d", scenes[0].SceneIndex, scenes[len(scenes)-1].SceneIndex)
}

// editPrompt states what is about to be charged for.
//
// No credit count: unlike an export, the price here depends on how much text
// the model writes and how long the speech comes out, and the backend does
// not quote it in advance. Naming the *kinds* of work being billed is what
// the CLI can say truthfully, and inventing a number would be worse than
// saying none — the whole gate rests on the user having seen real terms.
func editPrompt(sceneIndex int, scriptOnly bool) string {
	if scriptOnly {
		return i18n.T("edit.confirm_prompt_script_only", sceneIndex)
	}
	return i18n.T("edit.confirm_prompt", sceneIndex)
}

// editResumeCommand spells out the line that proceeds, with the script
// quoted so it survives being copied into a shell.
func editResumeCommand(taskID int64, sessionID string, sceneIndex int, script string, scriptOnly bool, token string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vk video edit %d --session-id %s --scene %d --script %s", taskID, sessionID, sceneIndex, shellQuote(script))
	if scriptOnly {
		b.WriteString(" --script-only")
	}
	fmt.Fprintf(&b, " --confirm %s", token)
	return b.String()
}

// shellQuote wraps s in single quotes, the one form with no escapes inside
// it. A narration line can hold anything a person might say — quotes,
// newlines, a stray backslash — and a resume command that only works for
// well-behaved text would fail exactly where it matters most.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() {
	editCmd.Flags().StringVar(&flagEditSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
	// No backticks in these descriptions: cobra reads the first backquoted
	// span as the flag's argument placeholder, so "as numbered by `vk video
	// script`" renders the usage line as `--scene vk video script` instead
	// of `--scene int`.
	editCmd.Flags().IntVar(&flagEditScene, "scene", 0, "which shot to change, as numbered by 'vk video script' (from 1)")
	editCmd.Flags().StringVar(&flagEditScript, "script", "", "the new narration for that shot")
	editCmd.Flags().BoolVar(&flagEditScriptOnly, "script-only", false, "regenerate the voice-over only; leave layout and background image untouched (cheaper)")
	editCmd.Flags().BoolVarP(&flagEditYes, "yes", "y", false, "skip confirmation prompt")
	editCmd.Flags().StringVar(&flagEditConfirm, "confirm", "", "action_id from a previously blocked run, once the user has agreed to the spend")
	editCmd.Flags().StringVar(&flagEditPreviewDir, "preview-dir", "", i18n.T("create.flag.preview_dir"))
}
