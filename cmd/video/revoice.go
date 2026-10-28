package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/video/snapshot"
	"github.com/vibeknow/cli/internal/voiceref"
)

var (
	flagRevoiceSessionID  string
	flagRevoiceVoice      string
	flagRevoiceYes        bool
	flagRevoiceConfirm    string
	flagRevoiceAsync      bool
	flagRevoicePreviewDir string
)

// revoicePollInterval is how often the job is re-read while it runs.
const revoicePollInterval = 3 * time.Second

var revoiceCmd = &cobra.Command{
	Use:   "revoice [task_id]",
	Short: "make a new version of a finished video with another voice (bills)",
	Args:  cobra.MaximumNArgs(1),
	Long: `Re-narrate a finished video with a different voice, keeping its script,
storyboard and visuals.

The result is a **new work** in a new session under the same task; the
original is left as it is. The output names both, and the new pair is
recorded in the run ledger — so ` + "`vk video status <task_id>`" + ` with no
--session-id now resolves to the new version. Pass the original
--session-id to address the original.

The new voice must speak the video's language; ` + "`vk voice list --language`" + `
filters by it.

**This bills**, by actual use: speech synthesis for the whole narration, and
lip-sync again when the video has a presenter. The backend quotes nothing in
advance, so the confirmation names what is billed rather than a figure.
Without a terminal the decision is handed back as exit 8; see
` + "`vk video export`" + ` for the same gate.

**Re-running the same command is safe.** A request is keyed on the work, its
current version and the voice, so a resend attaches to the run it already
started instead of starting a second one. A run that is still going is
watched again, one that finished is reported, and one that failed or ran out
of credits is retried — which bills again, so that case asks first.`,
	Example: `  vk voice list --language zh-CN
  vk video revoice 42 --voice 3
  vk video revoice 42 --voice <speech_voice_id> --confirm act_9f2c...
  vk video revoice 42 --voice 3 --yes --async`,
	RunE: runRevoice,
}

func runRevoice(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(flagRevoiceVoice) == "" {
		return clerr.Validation(i18n.T("revoice.err.voice_required")).
			WithHint(i18n.T("revoice.hint.voice_list"))
	}
	ctx := cmd.Context()
	taskID, sessionID, err := resolveTarget(ctx, args, flagRevoiceSessionID)
	if err != nil {
		return err
	}
	c, err := newFiglensClient()
	if err != nil {
		return err
	}
	ch, err := cmdutil.NewRunChannel(cmd, flagRevoicePreviewDir)
	if err != nil {
		return clerr.Validation(err.Error())
	}

	work, err := c.GetWorkBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	if taskID == 0 {
		taskID = work.TaskID
	}
	voice, err := voiceref.Resolve(ctx, flagRevoiceVoice)
	if err != nil {
		return err
	}
	capability, err := c.GetRevoiceCapability(ctx, work.ID)
	if err != nil {
		return err
	}

	// A run this exact request already started — same work, same version,
	// same voice — is attached to rather than started again.
	job, err := findRevoiceJob(ctx, c, taskID, work.ID, capability.SourceRevision, voice)
	if err != nil {
		return err
	}

	switch {
	case job != nil && !job.Retryable():
		if job.Status == figlens.RevoiceCanceled {
			return clerr.New(i18n.T("revoice.err.canceled")).WithCode(5)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("revoice.attached", job.SessionID))

	case job != nil:
		// Failed, paused or out of credits: going again is a new spend.
		ok, err := revoiceGate(cmd, taskID, sessionID, voice, capability, work)
		if err != nil || !ok {
			return err
		}
		if job, err = c.RetryRevoice(ctx, job.WorkID); err != nil {
			return err
		}

	default:
		if !capability.Supported {
			// The backend's own sentence: the work is still generating, is
			// being edited or exported, or predates the data a re-voice
			// copies. None of it is a mistake on the command line.
			return clerr.New(capability.Reason).WithCode(5).
				WithHint(i18n.T("revoice.hint.unsupported"))
		}
		if voice == capability.CurrentVoiceID {
			return clerr.Validation(i18n.T("revoice.err.same_voice"))
		}
		ok, err := revoiceGate(cmd, taskID, sessionID, voice, capability, work)
		if err != nil || !ok {
			return err
		}
		key := revoiceKey(work.ID, capability.SourceRevision, voice)
		if job, err = c.CreateRevoice(ctx, work.ID, capability.SourceRevision, voice, key); err != nil {
			return err
		}
	}

	recordRevoiceJob(job)
	if flagRevoiceAsync {
		return renderRevoiceSubmitted(cmd, *job, sessionID)
	}

	final, err := waitRevoice(ctx, cmd, ch, c, *job)
	if err != nil {
		return err
	}
	return finishRevoice(ctx, cmd, ch, c, final, sessionID)
}

func revoiceGate(cmd *cobra.Command, taskID int64, sessionID, voice string, cap *figlens.RevoiceCapability, work *figlens.Work) (bool, error) {
	prompt := i18n.T("revoice.confirm_prompt", voice, cap.LockedLanguage)
	if cap.HasAvatar {
		prompt += " " + i18n.T("revoice.confirm_avatar")
	}
	prompt += " " + i18n.T("revoice.confirm_tail")
	info := map[string]any{"language": cap.LockedLanguage}
	if work.Duration > 0 {
		// The narration length is what speech is billed against. It is a
		// fact about the work, shown so the user can size the spend; the
		// source_revision in the payload already pins the version.
		info["duration_sec"] = work.Duration / 1000
	}
	ok, err := cmdutil.Gate(cmd, cmdutil.GateOptions{
		Type:    cmdutil.RevoiceActionType,
		Payload: cmdutil.RevoiceActionPayload(sessionID, cap.SourceRevision, voice, cap.HasAvatar),
		Context: info,
		Prompt:  prompt,
		Yes:     flagRevoiceYes,
		Token:   flagRevoiceConfirm,
		ResumeCommand: func(token string) string {
			return fmt.Sprintf("vk video revoice %d --session-id %s --voice %s --confirm %s", taskID, sessionID, voice, token)
		},
	})
	if err == nil && !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("revoice.cancelled"))
	}
	return ok, err
}

// revoiceKey derives the idempotency key from what the request is about, so
// the same request always carries the same key and a resend cannot start a
// second billed run.
func revoiceKey(workID int64, revision, voice string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", workID, revision, voice)))
	return "vk-revoice-" + hex.EncodeToString(sum[:20])
}

// findRevoiceJob returns the most recent job for exactly this request, or
// nil. Matching on the revision as well as the voice means a work edited
// since is a new request, not a resend of the old one.
func findRevoiceJob(ctx context.Context, c *figlens.Client, taskID, workID int64, revision, voice string) (*figlens.RevoiceJob, error) {
	if taskID == 0 {
		return nil, nil
	}
	all, err := c.ListRevoiceJobs(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var found *figlens.RevoiceJob
	for i := range all {
		j := all[i]
		if j.SourceWorkID == workID && j.SourceRevision == revision && j.VoiceID == voice {
			found = &all[i]
		}
	}
	return found, nil
}

// waitRevoice re-reads the job until it settles. The job row, not the event
// stream, is the record of how a re-voice ended: it is what the backend
// retries from and what carries the failure reason.
func waitRevoice(ctx context.Context, cmd *cobra.Command, ch *cmdutil.RunChannel, c *figlens.Client, job figlens.RevoiceJob) (figlens.RevoiceJob, error) {
	lastStage := ""
	for {
		all, err := c.ListRevoiceJobs(ctx, job.TaskID)
		if err != nil {
			return job, err
		}
		for _, j := range all {
			if j.ID == job.ID {
				job = j
			}
		}
		if job.Stage != lastStage && job.Stage != "" {
			lastStage = job.Stage
			if ch.Structured() {
				ch.Emit(map[string]any{"type": "revoice.progress", "status": job.Status, "stage": job.Stage, "session_id": job.SessionID})
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("revoice.stage", job.Stage))
			}
		}
		switch job.Status {
		case figlens.RevoiceQueued, figlens.RevoiceRunning:
		default:
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(revoicePollInterval):
		}
	}
}

// finishRevoice turns a settled job into an exit code and, on success, the
// new work's snapshot.
func finishRevoice(ctx context.Context, cmd *cobra.Command, ch *cmdutil.RunChannel, c *figlens.Client, job figlens.RevoiceJob, sourceSession string) error {
	switch job.Status {
	case figlens.RevoiceCompleted:
	case figlens.RevoiceNeedsBudget:
		noteJob(job.TaskID, job.SessionID, func(r *jobs.Record) { r.Status = jobs.StatusFailed; r.Error = job.Error })
		return clerr.New(revoiceMessage(job)).WithCode(5).WithHint(i18n.T("revoice.hint.needs_budget"))
	case figlens.RevoicePaused:
		return clerr.New(revoiceMessage(job)).WithCode(6).WithHint(i18n.T("revoice.hint.rerun"))
	default:
		noteJob(job.TaskID, job.SessionID, func(r *jobs.Record) { r.Status = jobs.StatusFailed; r.Error = job.Error })
		return clerr.New(revoiceMessage(job)).WithCode(5).WithHint(i18n.T("revoice.hint.rerun"))
	}

	w, err := c.GetWorkBySession(ctx, job.SessionID)
	if err != nil {
		return err
	}
	s := snapshot.Build(snapshot.BuildInput{
		TaskID: job.TaskID, SessionID: job.SessionID, Work: w, ShareBase: cmdutil.ShareBaseURL(),
	})
	noteJob(job.TaskID, job.SessionID, func(r *jobs.Record) {
		r.Status = jobs.StatusSucceeded
		r.WorkID = s.WorkID
		r.Title = s.Title
		r.ShareURL = s.Preview.ShareURL
	})
	cmdutil.DeliverWorkArtifacts(ctx, ch.Previews, c, w)

	format, _ := cmd.Flags().GetString("output")
	if format == output.FormatJSON || format == output.FormatNDJSON {
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		m["revoice"] = revoiceInfo(job, sourceSession)
		return output.NewJSON(cmd.OutOrStdout()).Object(m)
	}
	snapshot.RenderText(cmd.OutOrStdout(), cmd.ErrOrStderr(), s)
	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("revoice.done", sourceSession))
	return nil
}

func revoiceMessage(j figlens.RevoiceJob) string {
	if msg := strings.TrimSpace(j.Error); msg != "" {
		return msg
	}
	return i18n.T("revoice.err.status", j.Status)
}

// revoiceInfo says which work this one was made from, so a caller holding
// only the result can still find the original.
func revoiceInfo(j figlens.RevoiceJob, sourceSession string) map[string]any {
	return map[string]any{
		"source_session_id": sourceSession,
		"source_work_id":    j.SourceWorkID,
		"voice_id":          j.VoiceID,
		"voice_name":        j.VoiceName,
		"language":          j.Language,
		"status":            j.Status,
	}
}

func renderRevoiceSubmitted(cmd *cobra.Command, j figlens.RevoiceJob, sourceSession string) error {
	resume := fmt.Sprintf("vk video revoice %d --session-id %s --voice %s", j.TaskID, sourceSession, j.VoiceID)
	format, _ := cmd.Flags().GetString("output")
	if format == output.FormatJSON || format == output.FormatNDJSON {
		return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
			"task_id":    j.TaskID,
			"session_id": j.SessionID,
			"work_id":    j.WorkID,
			"status":     j.Status,
			"revoice":    revoiceInfo(j, sourceSession),
			"next_actions": []map[string]string{{
				"command": resume,
				"purpose": "Watch the re-voice until it finishes; re-running attaches to this run and starts nothing new",
			}},
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "task_id=%d\nsession_id=%s\n", j.TaskID, j.SessionID)
	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("revoice.async_hint", resume))
	return nil
}

// recordRevoiceJob adds the new run to the ledger. Best-effort, like every
// ledger write: a re-voice that started must not be reported as a failure
// because a local index could not be written.
func recordRevoiceJob(j *figlens.RevoiceJob) {
	if j == nil || j.SessionID == "" {
		return
	}
	if _, ok, _ := jobs.GetBySession(j.SessionID); ok {
		return
	}
	if err := jobs.Append(jobs.Record{
		TaskID: j.TaskID, SessionID: j.SessionID, WorkID: j.WorkID,
		Status: jobs.StatusRunning, Source: "revoice", Engine: "pipeline",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write the local run ledger (%v); keep the task_id/session_id printed below\n", err)
	}
}

func init() {
	revoiceCmd.Flags().StringVar(&flagRevoiceSessionID, "session-id", "", "session ID of the work to re-voice (default: looked up in the local run ledger)")
	revoiceCmd.Flags().StringVar(&flagRevoiceVoice, "voice", "", i18n.T("revoice.flag.voice"))
	revoiceCmd.Flags().BoolVarP(&flagRevoiceYes, "yes", "y", false, "skip the confirmation: only when the user has already authorised this spend")
	revoiceCmd.Flags().StringVar(&flagRevoiceConfirm, "confirm", "", "action_id from a previously blocked run, once the user has agreed")
	revoiceCmd.Flags().BoolVar(&flagRevoiceAsync, "async", false, "return once the run has started instead of waiting for it")
	revoiceCmd.Flags().StringVar(&flagRevoicePreviewDir, "preview-dir", "", "write the new work's cover here and announce it as a resource_ready event")
}
