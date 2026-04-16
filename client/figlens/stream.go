package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
}

type StreamEvent struct {
	Type      string
	Stage     string
	Node      string
	Message   string
	SessionID string
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
}

type processLog struct {
	StepID  string `json:"step_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// mapSSECode maps a backend envelope code from an SSE payload to a CLI error code string.
func mapSSECode(code int) string {
	switch code {
	case 100001:
		return "insufficient_credits"
	case 100002:
		return "freeze_not_found"
	case 100003:
		return "concurrent_work_limit"
	default:
		return "business_error"
	}
}

func (c *Client) StreamChat(ctx context.Context, params StreamParams, onEvent func(StreamEvent)) error {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/agent3forVideo/stream", params)
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
			onEvent(StreamEvent{Type: "task.failed", Message: fmt.Sprintf("[%s] %s", code, msg)})
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
			onEvent(StreamEvent{Type: "task.succeeded", SessionID: d.SessionID})

		case "error", "ERROR":
			msg := d.Message
			if msg == "" {
				msg = string(payload.Data)
			}
			onEvent(StreamEvent{Type: "task.failed", Message: msg})
		}
	}
}
