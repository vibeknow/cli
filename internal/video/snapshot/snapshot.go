// Package snapshot owns the unified preview+export state shape returned
// by vk create, vk video status, vk video export, and vk video
// export-status. One source of truth keeps text and JSON output in sync
// across the four commands.
package snapshot

import "strings"

// Snapshot mirrors the JSON shape documented in
// docs/superpowers/specs/2026-04-22-two-stage-video-design.md.
type Snapshot struct {
	TaskID      int64    `json:"task_id"`
	SessionID   string   `json:"session_id"`
	WorkID      int64    `json:"work_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	DurationMs  int64    `json:"duration_ms,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	Preview     Preview  `json:"preview"`
	Export      Export   `json:"export"`
	NextActions []Action `json:"next_actions"`
}

type Preview struct {
	Ready         bool   `json:"ready"`
	ShareURL      string `json:"share_url,omitempty"`
	HTMLSignedURL string `json:"html_signed_url,omitempty"`
}

// Export.Status is one of: idle, running, succeeded, failed.
type Export struct {
	Status         string `json:"status"`
	ExportTaskID   string `json:"export_task_id,omitempty"`
	Progress       int    `json:"progress,omitempty"`
	ProgressMsg    string `json:"progress_msg,omitempty"`
	VideoPath      string `json:"video_path,omitempty"`
	VideoSignedURL string `json:"video_signed_url,omitempty"`
	Error          string `json:"error,omitempty"`
}

type Action struct {
	Command string `json:"command"`
	Purpose string `json:"purpose"`
}

const (
	StatusIdle      = "idle"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// ShareURL joins base + token into the shareable preview page URL.
// Empty token returns "" so callers can omit the field from output.
func ShareURL(base, token string) string {
	if token == "" {
		return ""
	}
	if base == "" {
		base = "https://vibeknow.com/share"
	}
	return strings.TrimRight(base, "/") + "/" + token
}
