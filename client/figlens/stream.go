package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/sse"
	"github.com/vibeknow/cli/internal/stage"
)

type StreamParams struct {
	TaskID      int64  `json:"task_id"`
	SessionID   string `json:"session_id"`
	Query       string `json:"query"`
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VoiceID     string `json:"voice_id,omitempty"`
	BGMEnabled  bool   `json:"bgm_enabled,omitempty"`
	Aspect      string `json:"aspect,omitempty"`
	VideoKind   string `json:"video_kind,omitempty"`
	Engine      Engine `json:"-"` // selects endpoint, never emitted in body
}

type StreamEvent struct {
	Type      string
	Code      string // set on task.failed when payload carries an envelope code
	Status    string // set on node.progress: "start" / "success" / "error"
	Stage     string
	Node      string
	Message   string
	SessionID string
	// Set on task.succeeded. VideoURL is the playable HTML URL (v=3
	// `html_path`, v=2 `text`). DurationMs is the rendered video length
	// in milliseconds; backend only emits it on v=3 (in `data.duration_ms`),
	// so v=2 task.succeeded events carry DurationMs=0.
	VideoURL   string
	DurationMs int64
	// Set on task.failed. Derived from Code via httpclient.IsRetryableCode
	// because the backend's terminal SSE error event carries no retryable
	// flag of its own.
	Retryable bool
}

type ssePayload struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type sseData struct {
	Type      string          `json:"type"`
	Log       json.RawMessage `json:"log,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Message   string          `json:"message,omitempty"`
	// aim_result terminal-event fields. The two engines disagree on
	// where the playable URL lives — see resolveVideoURL.
	HtmlPath string          `json:"html_path,omitempty"`  // v=3 pipeline
	Text     string          `json:"text,omitempty"`       // v=2 agent (also free text on v=3, ignored there)
	DataMap  json.RawMessage `json:"data,omitempty"`       // v=3 metadata bag, contains duration_ms etc.
}

// aimResultData captures the v=3 metadata bag. Other fields exist
// (themeId, fps, coverUrl, scenes, …) but the CLI only forwards
// duration_ms today — additions are cheap when consumers need them.
type aimResultData struct {
	DurationMs int64 `json:"duration_ms"`
}

type processLog struct {
	StepID  string `json:"step_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// mapSSECode maps an SSE envelope code to a CLI error code label, delegating
// to httpclient.MapBusinessCode so the two transports never diverge.
func mapSSECode(code int) string {
	if label, ok := httpclient.MapBusinessCode(code); ok {
		return label
	}
	return "business_error"
}

// resolveVideoURL extracts the playable HTML URL from an aim_result payload.
// The two engines disagree on which field holds it:
//   - v=3 pipeline: `html_path` (preferred — engine emits a structured URL).
//   - v=2 agent:    `text` (the agent only has one free-form result field;
//     it puts the HTML URL there directly).
//
// Picking HtmlPath first means v=3 wins cleanly when both happen to be set,
// which they shouldn't be — but we defend against the cross-engine drift
// case rather than assume the backend stays in its lane.
func resolveVideoURL(d sseData) string {
	if d.HtmlPath != "" {
		return d.HtmlPath
	}
	return d.Text
}

func (c *Client) StreamChat(ctx context.Context, params StreamParams, onEvent func(StreamEvent)) error {
	resp, err := c.http.DoRaw(ctx, "POST", params.Engine.StreamPath(), params)
	if err != nil {
		return fmt.Errorf("stream chat: %w", err)
	}
	defer resp.Body.Close()

	reader := sse.NewReader(resp.Body)

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("sse read: %w", err)
		}

		data := strings.TrimSpace(ev.Data)

		if data == "[DONE]" {
			return nil
		}

		var payload ssePayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		// Check for business errors in the SSE payload (e.g. insufficient credits).
		// Backend uses code 0 or 200 for success; anything else is an error.
		if payload.Code != 0 && payload.Code != 200 {
			msg := "business error"
			if len(payload.Data) > 0 {
				var d sseData
				if json.Unmarshal(payload.Data, &d) == nil && d.Message != "" {
					msg = d.Message
				}
			}
			code := mapSSECode(payload.Code)
			onEvent(StreamEvent{
				Type:      "task.failed",
				Code:      code,
				Message:   msg,
				Retryable: httpclient.IsRetryableCode(code),
			})
			return nil
		}

		var d sseData
		if err := json.Unmarshal(payload.Data, &d); err != nil {
			continue
		}

		switch d.Type {
		case "process":
			var log processLog
			if err := json.Unmarshal(d.Log, &log); err != nil {
				continue
			}
			if log.StepID == "" {
				// v=2 agent path: free-form progress, no node graph.
				onEvent(StreamEvent{
					Type:    "node.progress",
					Status:  log.Status,
					Message: log.Message,
				})
				continue
			}
			if !stage.IsKnownNode(log.StepID) {
				continue
			}
			displayName := stage.DisplayName(log.StepID)
			stageName, _ := stage.FromNode(log.StepID)

			switch log.Status {
			case "start":
				onEvent(StreamEvent{
					Type: "node.started", Stage: stageName,
					Node: displayName, Message: log.Message,
				})
			case "success":
				onEvent(StreamEvent{
					Type: "node.succeeded", Stage: stageName,
					Node: displayName, Message: log.Message,
				})
			case "error":
				onEvent(StreamEvent{
					Type: "node.failed", Stage: stageName,
					Node: displayName, Message: log.Message,
				})
			}

		case "aim_result":
			ev := StreamEvent{Type: "task.succeeded", SessionID: d.SessionID}
			ev.VideoURL = resolveVideoURL(d)
			if len(d.DataMap) > 0 {
				var ar aimResultData
				if json.Unmarshal(d.DataMap, &ar) == nil {
					ev.DurationMs = ar.DurationMs
				}
			}
			onEvent(ev)

		case "error", "ERROR":
			msg := d.Message
			if msg == "" {
				msg = string(payload.Data)
			}
			// Plain `error` SSE never carries a code; Retryable defaults
			// to false so consumers do not silently re-run unbounded.
			onEvent(StreamEvent{Type: "task.failed", Message: msg})
		}
	}
}
