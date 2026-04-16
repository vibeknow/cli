package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/shiliu-ai/vibeknow-cli/internal/sse"
	"github.com/shiliu-ai/vibeknow-cli/internal/stage"
)

type StreamParams struct {
	TaskID      int    `json:"task_id"`
	SessionID   string `json:"session_id"`
	Query       string `json:"query"`
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VoiceID     string `json:"voice_id,omitempty"`
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

func (c *Client) StreamChat(ctx context.Context, params StreamParams, onEvent func(StreamEvent)) error {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/agent3forVideo/stream", params)
	if err != nil {
		return fmt.Errorf("stream chat: %w", err)
	}
	defer resp.Body.Close()

	reader := sse.NewReader(resp.Body)
	stageStarted := map[string]bool{}

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
			stageName, ok := stage.FromNode(log.StepID)
			if !ok {
				continue
			}

			switch log.Status {
			case "start":
				if !stageStarted[stageName] {
					stageStarted[stageName] = true
					onEvent(StreamEvent{
						Type: "stage.started", Stage: stageName,
						Node: log.StepID, Message: log.Message,
					})
				}
			case "success":
				onEvent(StreamEvent{
					Type: "stage.succeeded", Stage: stageName,
					Node: log.StepID, Message: log.Message,
				})
			case "error":
				onEvent(StreamEvent{
					Type: "stage.failed", Stage: stageName,
					Node: log.StepID, Message: log.Message,
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
