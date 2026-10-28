package figlens

import (
	"context"
	"fmt"
	"strconv"
)

// RevoiceCapability is GET /v1/works/revoiceCapability: whether a work can be
// re-voiced, and the fingerprint a request must quote.
type RevoiceCapability struct {
	Supported bool `json:"supported"`
	// Reason is the backend's own sentence for why not, already localised.
	Reason string `json:"reason,omitempty"`
	// LockedLanguage is the work's narration language. The new voice must
	// speak it; the backend refuses any other.
	LockedLanguage string `json:"locked_language"`
	HasAvatar      bool   `json:"has_avatar"`
	// SourceRevision fingerprints the work as it is now. A request quoting
	// a stale one is refused, so a re-voice never copies a version of the
	// work the caller did not look at.
	SourceRevision string `json:"source_revision"`
	CurrentVoiceID string `json:"current_voice_id"`
}

// RevoiceJob is one re-voice run. It produces a new work, in a new session
// under the same task; the source work is left untouched.
type RevoiceJob struct {
	ID             string `json:"id"`
	TaskID         int64  `json:"task_id"`
	WorkID         int64  `json:"work_id"`
	SessionID      string `json:"session_id"`
	SourceWorkID   int64  `json:"source_work_id"`
	SourceRevision string `json:"source_revision"`
	VoiceID        string `json:"voice_id"`
	VoiceName      string `json:"voice_name"`
	Language       string `json:"language"`
	// Status is queued | running | completed | failed | needs_budget |
	// paused | canceled. Stage is a finer-grained step name while running.
	Status string `json:"status"`
	Stage  string `json:"stage"`
	Error  string `json:"error,omitempty"`
}

// Revoice job states the CLI acts on.
const (
	RevoiceQueued      = "queued"
	RevoiceRunning     = "running"
	RevoiceCompleted   = "completed"
	RevoiceFailed      = "failed"
	RevoiceNeedsBudget = "needs_budget"
	RevoicePaused      = "paused"
	RevoiceCanceled    = "canceled"
)

// Retryable reports whether /works/revoiceRetry accepts the job.
func (j RevoiceJob) Retryable() bool {
	return j.Status == RevoiceFailed || j.Status == RevoiceNeedsBudget || j.Status == RevoicePaused
}

func (c *Client) GetRevoiceCapability(ctx context.Context, workID int64) (*RevoiceCapability, error) {
	var out RevoiceCapability
	path := "/v1/works/revoiceCapability?work_id=" + strconv.FormatInt(workID, 10)
	if err := c.http.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("revoice capability: %w", err)
	}
	return &out, nil
}

type createRevoiceRequest struct {
	WorkID         int64  `json:"work_id"`
	SourceRevision string `json:"source_revision"`
	VoiceID        string `json:"voice_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CreateRevoice starts a re-voice. The idempotency key makes a resend safe:
// the same key with the same work, revision and voice returns the job it
// already created instead of starting — and billing — a second one.
func (c *Client) CreateRevoice(ctx context.Context, workID int64, revision, voiceID, key string) (*RevoiceJob, error) {
	var out RevoiceJob
	body := createRevoiceRequest{WorkID: workID, SourceRevision: revision, VoiceID: voiceID, IdempotencyKey: key}
	if err := c.http.Do(ctx, "POST", "/v1/works/revoice", body, &out); err != nil {
		return nil, fmt.Errorf("start revoice: %w", err)
	}
	return &out, nil
}

// ListRevoiceJobs returns every re-voice started under a task, oldest first.
func (c *Client) ListRevoiceJobs(ctx context.Context, taskID int64) ([]RevoiceJob, error) {
	var out []RevoiceJob
	path := "/v1/works/revoiceJobs?task_id=" + strconv.FormatInt(taskID, 10)
	if err := c.http.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("list revoice jobs: %w", err)
	}
	return out, nil
}

// RetryRevoice re-queues a failed, paused or budget-blocked job. workID is
// the job's own (new) work, not the source.
func (c *Client) RetryRevoice(ctx context.Context, workID int64) (*RevoiceJob, error) {
	var out RevoiceJob
	body := map[string]int64{"work_id": workID}
	if err := c.http.Do(ctx, "POST", "/v1/works/revoiceRetry", body, &out); err != nil {
		return nil, fmt.Errorf("retry revoice: %w", err)
	}
	return &out, nil
}
