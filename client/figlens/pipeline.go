package figlens

import (
	"context"
	"fmt"
)

// Pause and Resume are the user-facing stop/continue for a running pipeline.
//
// There is also a /videos/cancel on the backend, and it is not this: that one
// is registered under /internal/v1 behind a static service token, so it is a
// platform-to-platform call rather than something a user's credential can
// make. Pausing is what "stop this" means for an account holder.
type pipelineSessionRequest struct {
	SessionID string `json:"session_id"`
}

// PipelineResumeMode says which of the two things resume just did.
type PipelineResumeMode string

const (
	// ResumeModePaused continued a run the user had paused.
	ResumeModePaused PipelineResumeMode = "paused_resume"
	// ResumeModeFailedRetry restarted a *failed* run from its last
	// checkpoint rather than from the beginning. The backend reopens the
	// original bill for it, so this is materially cheaper than creating the
	// video again — which is the only other way to react to a failure.
	ResumeModeFailedRetry PipelineResumeMode = "failed_checkpoint_retry"
)

type pipelineResumeResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Mode      string `json:"mode"`
}

// PausePipeline stops a run that is currently generating.
//
// Only a generating run can be paused; the backend rejects anything else,
// including a run that has already finished or failed.
func (c *Client) PausePipeline(ctx context.Context, sessionID string) error {
	if err := c.http.Do(ctx, "POST", "/v1/pipeline/pause",
		pipelineSessionRequest{SessionID: sessionID}, nil); err != nil {
		return fmt.Errorf("pause pipeline: %w", err)
	}
	return nil
}

// ResumePipeline continues a paused run, or retries a failed one from its
// checkpoint, and reports which of the two happened.
//
// The backend refuses in three cases worth telling apart from a transient
// failure, because none of them can be fixed by trying again: the run was
// made with the agent engine (which keeps no checkpoint), it was stopped by
// the provider's content policy (the same inputs would be stopped again), or
// it is in a state that is neither paused nor failed.
func (c *Client) ResumePipeline(ctx context.Context, sessionID string) (PipelineResumeMode, error) {
	var resp pipelineResumeResponse
	if err := c.http.Do(ctx, "POST", "/v1/pipeline/resume",
		pipelineSessionRequest{SessionID: sessionID}, &resp); err != nil {
		return "", fmt.Errorf("resume pipeline: %w", err)
	}
	return PipelineResumeMode(resp.Mode), nil
}
