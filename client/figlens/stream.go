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
	// ScriptLock mirrors InitTaskParams.ScriptLock and must be repeated
	// here: the pipeline entry reads it off the stream request, not off the
	// task, to decide whether to skip script writing. Omitting it makes the
	// backend fall through to the standard write-a-script line — silently,
	// with the user's own script demoted to reference material.
	ScriptLock bool `json:"script_lock,omitempty"`
	// PageCount is image-mode-only: the exact page count for generated
	// images (image-generation cost scales with it). 0 lets the
	// storyboard decide.
	PageCount int `json:"page_count,omitempty"`
	// Theme is a style catalog ID from `vk theme list` (or a ua_<assetId>
	// custom-theme token). Empty lets the pipeline auto-select. Suite
	// membership must match VideoKind — the backend hard-rejects an
	// image-suite theme outside image mode rather than degrading silently.
	Theme string `json:"theme,omitempty"`
	// Language is the output locale for script + TTS (zh-CN, en-US, es-ES,
	// fr-FR, pt-BR, ja-JP, ko-KR). Only the v=3 pipeline reads it; empty
	// (or any unrecognized value) falls back to the deployment default.
	Language string `json:"language,omitempty"`
	// Avatar selects a talking-head presenter: "sys_<id>" (public preset,
	// see `vk avatar list`) or "ua_<id>" (own activated asset). The wire
	// keys are camelCase — the one part of this request that is — because
	// the backend binds them that way. Validated hard at the stream entry
	// (unknown/unactivated refs are a 400, never a silent skip).
	Avatar string `json:"avatar,omitempty"`
	// AvatarPosition is a corner preset: top-left / top-right /
	// bottom-left / bottom-right. Empty falls back server-side to the
	// user's saved preference, then the asset default, then top-left.
	AvatarPosition string `json:"avatarPosition,omitempty"`
	// AvatarHeightPx is the circle-window diameter at a 1080p base,
	// 120–480. 0 falls back like AvatarPosition (default 240).
	AvatarHeightPx float64 `json:"avatarHeightPx,omitempty"`
	// SelectedImageIndexes mirrors InitTaskParams; sending it on the stream
	// too covers the second-generation path where no init call happens.
	SelectedImageIndexes []int  `json:"selected_image_indexes,omitempty"`
	Engine               Engine `json:"-"` // selects endpoint, never emitted in body
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
	// Metrics carries the node's real output numbers on node.succeeded
	// (e.g. chapters, script_chars, duration_sec). Nil for most events;
	// forwarded to structured output only.
	Metrics map[string]any
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
	// Msg carries the stream-done sentinel: the backend wraps "[DONE]"
	// inside the ordinary {code,data} envelope (data.msg == "[DONE]"),
	// it is not a bare SSE data line. EOF remains the fallback terminator
	// for deployments that close the connection without one.
	Msg string `json:"msg,omitempty"`
	// Current backend (assistant_event rework) nests terminal payloads:
	// aim_result fields under answer_done, failure details under error.
	AnswerDone *sseAnswerDone `json:"answer_done,omitempty"`
	Err        *sseError      `json:"error,omitempty"`
	// Legacy flat aim_result fields (pre-rework deployments). The two
	// engines disagree on where the playable URL lives — see resolveVideoURL.
	HtmlPath string          `json:"html_path,omitempty"` // v=3 pipeline
	Text     string          `json:"text,omitempty"`      // v=2 agent (also free text on v=3, ignored there)
	DataMap  json.RawMessage `json:"data,omitempty"`      // v=3 metadata bag, contains duration_ms etc.
}

// sseAnswerDone mirrors the backend's AssistantAnswerDone: html_path holds
// the playable URL on v=3, text holds it on v=2 (where text doubles as a
// human-readable completion message on v=3), data is the v=3 metadata bag.
type sseAnswerDone struct {
	Text     string          `json:"text"`
	HtmlPath string          `json:"html_path,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// sseError mirrors the backend's AssistantError carried by `error` events.
type sseError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
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
	// Metrics carries real node outputs on success events (chapters,
	// script_chars, cover_count, duration_sec, bg_count — the key set is
	// per-node and open-ended). Forwarded verbatim to structured consumers.
	Metrics map[string]any `json:"metrics,omitempty"`
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
// The current backend nests it under answer_done; older deployments emit it
// flat. The two shapes never mix on the wire, so each is resolved on its own
// — the nested branch must not fall back to flat fields. Within either shape
// the two engines disagree on which field holds the URL:
//   - v=3 pipeline: `html_path` (preferred — engine emits a structured URL).
//   - v=2 agent:    `text` (the agent only has one free-form result field;
//     it puts the HTML URL there directly).
func resolveVideoURL(d sseData) string {
	if d.AnswerDone != nil {
		if d.AnswerDone.HtmlPath != "" {
			return d.AnswerDone.HtmlPath
		}
		// On v=3 the nested text is a human-readable completion message
		// ("视频已生成"), which must not leak into a URL field — hence
		// the prefix gate on the v=2 fallback.
		if strings.HasPrefix(d.AnswerDone.Text, "http://") || strings.HasPrefix(d.AnswerDone.Text, "https://") {
			return d.AnswerDone.Text
		}
		return ""
	}
	if d.HtmlPath != "" {
		return d.HtmlPath
	}
	// Legacy flat `text` carried only the URL; accept it verbatim so
	// pre-rework v=2 deployments that returned object keys keep working.
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

		if d.Msg == "[DONE]" {
			return nil
		}

		switch d.Type {
		case "process":
			var log processLog
			if err := json.Unmarshal(d.Log, &log); err != nil {
				continue
			}
			if log.StepID == "" {
				// Free-form progress with no node attribution. Three
				// producers share this shape: the v=2 agent path (which has
				// no node graph at all), the v=3 run-started event ("开始
				// 处理…", status=start), and the v=3 stalled-run heartbeat
				// (status=pending).
				onEvent(StreamEvent{
					Type:    "node.progress",
					Status:  log.Status,
					Message: log.Message,
				})
				continue
			}
			if !stage.IsKnownNode(log.StepID) {
				// A node this build has never heard of. Dropping it made the
				// CLI go completely silent for the duration of that node —
				// and the backend has already renamed its graph once (see the
				// package comment on internal/stage), so this is a question of
				// when, not if.
				//
				// Forward it as free-form progress instead, the same shape the
				// v=2 agent path uses. The raw step_id is deliberately not
				// included: unrecognized wire names are exactly the ones that
				// may carry an internal model codename, which is why
				// nodeDisplayName exists. The backend's message is already
				// user-facing for every known node, so it is safe to show.
				onEvent(StreamEvent{
					Type:    "node.progress",
					Status:  log.Status,
					Message: log.Message,
				})
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
					Metrics: log.Metrics,
				})
			case "warning":
				// Non-fatal degradation (e.g. image mode falls back to a
				// placeholder for a failed page). The task still succeeds;
				// surface it so users learn why the output looks off.
				onEvent(StreamEvent{
					Type: "node.warning", Stage: stageName,
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
			dataMap := d.DataMap
			if d.AnswerDone != nil && len(d.AnswerDone.Data) > 0 {
				dataMap = d.AnswerDone.Data
			}
			if len(dataMap) > 0 {
				var ar aimResultData
				if json.Unmarshal(dataMap, &ar) == nil {
					ev.DurationMs = ar.DurationMs
				}
			}
			onEvent(ev)

		case "paused":
			// The run was paused (web editor's pause button, or a
			// multi-instance handover). Not terminal and not a failure:
			// the work sits in status=4 until resumed. The payload is a
			// process-log shape whose message says the plan was cancelled.
			msg := d.Message
			var log processLog
			if len(d.Log) > 0 && json.Unmarshal(d.Log, &log) == nil && log.Message != "" {
				msg = log.Message
			}
			onEvent(StreamEvent{Type: "task.paused", Message: msg})

		case "keepalive":
			// Real data-frame heartbeat (comment-frame heartbeats don't
			// survive some gateways). Carries nothing; deliberately ignored.

		case "error", "ERROR":
			msg := d.Message
			if d.Err != nil && d.Err.Message != "" {
				msg = d.Err.Message
			}
			if msg == "" {
				msg = string(payload.Data)
			}
			// Plain `error` SSE never carries a code; Retryable defaults
			// to false so consumers do not silently re-run unbounded.
			onEvent(StreamEvent{Type: "task.failed", Message: msg})
		}
	}
}
