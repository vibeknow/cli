package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/shiliu-ai/vibeknow-cli/internal/sse"
)

type OptimizeParams struct {
	KnowledgeID string `json:"knowledge_id"`
	DocID       string `json:"doc_id"`
	Query       string `json:"query,omitempty"`
}

type optimizePayload struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type optimizeData struct {
	Type        string          `json:"type"`
	AnswerDelta json.RawMessage `json:"answer_delta,omitempty"`
	AnswerDone  json.RawMessage `json:"answer_done,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	Message     string          `json:"message,omitempty"`
}

type answerText struct {
	Text string `json:"text"`
}

// FastQueryOptimize calls the fastQueryOptimize SSE endpoint and returns
// the full optimized prompt. onDelta is called with each streamed token
// fragment (may be nil if caller does not need incremental output).
func (c *Client) FastQueryOptimize(ctx context.Context, params OptimizeParams, onDelta func(string)) (string, error) {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/agent2forVideo/fastQueryOptimize", params)
	if err != nil {
		return "", fmt.Errorf("fast query optimize: %w", err)
	}
	defer resp.Body.Close()

	reader := sse.NewReader(resp.Body)
	var full strings.Builder

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("sse read: %w", err)
		}

		data := strings.TrimSpace(ev.Data)
		if data == "[DONE]" {
			break
		}

		var payload optimizePayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		var d optimizeData
		if err := json.Unmarshal(payload.Data, &d); err != nil {
			continue
		}

		switch d.Type {
		case "data":
			var delta answerText
			if err := json.Unmarshal(d.AnswerDelta, &delta); err == nil && delta.Text != "" {
				full.WriteString(delta.Text)
				if onDelta != nil {
					onDelta(delta.Text)
				}
			}
		case "aim_result":
			var done answerText
			if err := json.Unmarshal(d.AnswerDone, &done); err == nil && done.Text != "" {
				return done.Text, nil
			}
		case "error", "ERROR":
			msg := d.Message
			if msg == "" {
				var errObj answerText
				if json.Unmarshal(d.Error, &errObj) == nil {
					msg = errObj.Text
				}
			}
			if msg == "" {
				msg = string(payload.Data)
			}
			return "", fmt.Errorf("optimize failed: %s", msg)
		}
	}

	// If no aim_result was received, return whatever we accumulated.
	if full.Len() > 0 {
		return full.String(), nil
	}
	return "", fmt.Errorf("optimize: no result received")
}
