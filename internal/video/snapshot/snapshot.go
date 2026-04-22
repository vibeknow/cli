// Package snapshot owns the unified preview+export state shape returned
// by vk create, vk video status, vk video export, and vk video
// export-status. One source of truth keeps text and JSON output in sync
// across the four commands.
package snapshot

import (
	"fmt"
	"strings"

	"github.com/vibeknow/cli/client/figlens"
)

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

// BuildInput carries all raw API data needed to derive a Snapshot.
type BuildInput struct {
	TaskID       int64
	SessionID    string
	Work         *figlens.Work
	Export       *figlens.ExportResult
	ExportTaskID string
	ShareBase    string
}

// Build derives a Snapshot from raw API data, computing export status and
// next_actions in one place so all commands share the same logic.
func Build(in BuildInput) Snapshot {
	s := Snapshot{
		TaskID:    in.TaskID,
		SessionID: in.SessionID,
	}
	if in.Work != nil {
		s.WorkID = in.Work.ID
		s.Title = in.Work.Title
		s.DurationMs = in.Work.Duration
		s.CoverURL = in.Work.CoverURL
		s.Preview.Ready = in.Work.ShareToken != ""
		s.Preview.ShareURL = ShareURL(in.ShareBase, in.Work.ShareToken)
	}
	s.Export = deriveExport(in)
	s.NextActions = nextActions(s, in)
	return s
}

func deriveExport(in BuildInput) Export {
	e := Export{Status: StatusIdle, ExportTaskID: in.ExportTaskID}
	// Backend ExportResult is the most specific signal when present.
	if in.Export != nil {
		switch in.Export.Status {
		case "completed", "success", "succeeded":
			e.Status = StatusSucceeded
			e.VideoPath = in.Export.VideoPath
		case "failed", "error":
			e.Status = StatusFailed
			e.Error = in.Export.Error
		default:
			e.Status = StatusRunning
			e.Progress = in.Export.Progress
			e.ProgressMsg = in.Export.ProgressMsg
		}
	}
	// Fall back to the work row's flag/path when no ExportResult provided.
	if e.Status == StatusIdle && in.Work != nil {
		switch {
		case in.Work.VideoPath != "":
			e.Status = StatusSucceeded
			e.VideoPath = in.Work.VideoPath
		case in.Work.Exporting == 1:
			e.Status = StatusRunning
		}
	}
	return e
}

func nextActions(s Snapshot, in BuildInput) []Action {
	base := fmt.Sprintf("%d --session-id %s", in.TaskID, in.SessionID)
	switch {
	case !s.Preview.Ready:
		return []Action{{
			Command: "vk video wait " + base,
			Purpose: "Wait for the generation pipeline to finish",
		}}
	case s.Export.Status == StatusIdle:
		return []Action{{
			Command: "vk video export " + base,
			Purpose: "Render MP4 (several minutes, extra credits)",
		}}
	case s.Export.Status == StatusRunning:
		return []Action{{
			Command: "vk video status " + base,
			Purpose: "Poll export progress",
		}}
	case s.Export.Status == StatusSucceeded:
		return []Action{{
			Command: "vk video download " + base + " --output out.mp4",
			Purpose: "Download the rendered MP4",
		}}
	case s.Export.Status == StatusFailed:
		return []Action{{
			Command: "vk video export " + base,
			Purpose: "Retry export after previous failure",
		}}
	}
	return nil
}
