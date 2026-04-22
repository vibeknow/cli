// Package snapshot owns the unified preview+export state shape returned
// by vk create, vk video status, vk video export, and vk video
// export-status. One source of truth keeps text and JSON output in sync
// across the four commands.
package snapshot

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/output"
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

// RenderText writes a stable key=value block on w and advisory hint lines on
// errW. Ordering: IDs → title/duration → share_url (its own line) → blank →
// hints from next_actions. Agents parsing stdout can grep for share_url=.
func RenderText(w, errW io.Writer, s Snapshot) {
	fmt.Fprintf(w, "task_id=%d session_id=%s", s.TaskID, s.SessionID)
	if s.WorkID != 0 {
		fmt.Fprintf(w, " work_id=%d", s.WorkID)
	}
	fmt.Fprintln(w)
	if s.Title != "" {
		fmt.Fprintf(w, "title=%s\n", s.Title)
	}
	if s.DurationMs > 0 {
		fmt.Fprintf(w, "duration=%s\n", formatDuration(s.DurationMs))
	}
	if s.Preview.ShareURL != "" {
		fmt.Fprintf(w, "share_url=%s\n", s.Preview.ShareURL)
	}
	if s.Export.VideoPath != "" {
		fmt.Fprintf(w, "video_path=%s\n", s.Export.VideoPath)
	}
	if s.Export.VideoSignedURL != "" {
		fmt.Fprintf(w, "video_signed_url=%s\n", s.Export.VideoSignedURL)
	}
	for _, a := range s.NextActions {
		fmt.Fprintf(errW, "hint: %s — %s\n", a.Purpose, a.Command)
	}
}

// RenderJSON writes the snapshot as a JSON object via the shared output
// writer, which stamps schema_version and handles HTML-escaping policy.
// We round-trip through json.Marshal/Unmarshal because output.NewJSON's
// Object method takes map[string]any; this keeps the schema_version
// stamping behavior consistent with the rest of the CLI.
func RenderJSON(w io.Writer, s Snapshot) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	return output.NewJSON(w).Object(m)
}

func formatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	total := int(d.Seconds())
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	return fmt.Sprintf("%dm%02ds", total/60, total%60)
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
