package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/sse"
)

// Scene edit rule codes, as the backend routes them.
//
// Only two are used here. The rest of the range exists and is documented so
// the numbers in a request body are readable without the backend open:
//
//	0  no-op   — the scene is unchanged; the router skips it
//	1  normal  — narration, background image or slot text
//	2  delete  — drop the scene, then reindex the rest
//	3  relayout — swap the layout (requires a preview round trip first)
//	4  image2 text redraw
//	5  image2 redesign
const (
	SceneEditNoop   = 0
	SceneEditNormal = 1
)

// SceneEditSceneParams is one entry of the scenes array.
//
// There is no scene index in here, and that is not an omission: the backend
// identifies a scene by its *position*, `scene_index = i + 1`. A caller
// therefore has to send one entry per scene of the work, in order, marking
// the untouched ones SceneEditNoop. The backend rejects a short array
// outright ("scenes length mismatch"), which is the good outcome — the bad
// one would be a request that edits whichever scene happens to sit at that
// position.
type SceneEditSceneParams struct {
	Edit int `json:"edit"`
	// ScriptText is a pointer because the backend distinguishes "not sent"
	// from "sent as empty": omitted keeps the existing narration, empty
	// string is a caller trying to erase it.
	ScriptText *string `json:"scriptText,omitempty"`
}

// SceneEditParams is the /v1/scene/edit request body.
type SceneEditParams struct {
	SessionID string                 `json:"session_id"`
	Scenes    []SceneEditSceneParams `json:"scenes"`
	// ScriptOnly regenerates narration audio, timeline and presenter only:
	// no re-layout, no new content cards, no new background image. It is
	// the cheaper half of a script change — the layout keeps whatever it
	// had, which is right for a rewrite of similar length and wrong for one
	// that changes how much text there is.
	ScriptOnly bool `json:"script_only,omitempty"`
}

// SceneEditEvent is one observation from the edit stream.
type SceneEditEvent struct {
	// Type is "edit.started", "edit.progress" or "edit.succeeded".
	Type string
	// Status is the backend's own step status on progress events,
	// "success" or "fail".
	Status string
	// Message is the backend's human sentence for the step, already
	// localised server-side.
	Message string
	// PreviewURL is set on edit.succeeded: the playable HTML for the work
	// as it now stands.
	PreviewURL string
}

// sceneEditData covers all three shapes the backend puts inside the
// {code, data} envelope on this stream.
//
// Progress events are AssistantEvent (type + log). The terminal event is a
// bare {type: "edit_completed", data: {...}}. A pipeline failure arrives as
// ChatReplyContent — msg_type "ERROR" with the message in msg — inside a
// *code 200* envelope, so the envelope code cannot be used to detect it.
type sceneEditData struct {
	Type string `json:"type"`
	Log  *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"log,omitempty"`
	MsgType string          `json:"msg_type,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type sceneEditResult struct {
	HTMLPath string `json:"html_path"`
}

// StreamSceneEdit submits a scene edit and reports progress until the
// backend finishes or fails.
//
// Two refusals never reach the stream at all. Both the edit lock ("this work
// is already being edited") and the credit precheck are answered before the
// SSE headers go out, as an ordinary JSON envelope — on HTTP 200, because
// the gateway maps unrecognised business codes that way. Reading them as SSE
// would turn a precise, actionable code into "the stream ended early", so
// the content type is checked before a single event is parsed.
func (c *Client) StreamSceneEdit(ctx context.Context, params SceneEditParams, onEvent func(SceneEditEvent)) error {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/scene/edit", params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !isEventStream(resp) {
		return decodeSceneEditRefusal(resp)
	}

	reader := sse.NewReader(resp.Body)
	for {
		ev, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("sse read: %w", err)
		}

		raw := strings.TrimSpace(ev.Data)
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var payload ssePayload
		if json.Unmarshal([]byte(raw), &payload) != nil {
			continue
		}
		if payload.Code != 0 && payload.Code != 200 {
			return &errs.Object{
				SchemaVersion: "1",
				Code:          mapSSECode(payload.Code),
				Message:       sceneEditMessage(payload.Data),
				Retryable:     httpclient.IsRetryableCode(mapSSECode(payload.Code)),
			}
		}

		var d sceneEditData
		if json.Unmarshal(payload.Data, &d) != nil {
			continue
		}

		// A mid-stream failure. The backend restores the pre-edit version
		// when a node fails, so the usual outcome is "nothing happened"
		// rather than a half-applied edit — but that restore is
		// best-effort compensation, not a transaction, so the honest thing
		// to tell a caller is that the edit failed and the work is worth
		// re-reading. Returning an error is what stops it being reported
		// as an edit that landed.
		if strings.EqualFold(d.MsgType, "ERROR") {
			return &errs.Object{
				SchemaVersion: "1",
				Code:          "business_error",
				Message:       strings.TrimSpace(d.Msg),
			}
		}

		switch d.Type {
		case "edit_start":
			onEvent(SceneEditEvent{Type: "edit.started", Message: logMessage(d)})
		case "process":
			onEvent(SceneEditEvent{Type: "edit.progress", Status: logStatus(d), Message: logMessage(d)})
		case "edit_completed":
			var r sceneEditResult
			_ = json.Unmarshal(d.Data, &r)
			onEvent(SceneEditEvent{Type: "edit.succeeded", PreviewURL: r.HTMLPath})
			return nil
		}
	}
}

func logMessage(d sceneEditData) string {
	if d.Log == nil {
		return ""
	}
	return strings.TrimSpace(d.Log.Message)
}

func logStatus(d sceneEditData) string {
	if d.Log == nil {
		return ""
	}
	return strings.TrimSpace(d.Log.Status)
}

func isEventStream(resp *http.Response) bool {
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
}

// decodeSceneEditRefusal turns the pre-stream JSON envelope into the same
// error shape every other backend call produces, so `work_edit_busy` reaches
// the exit-code table (4, retryable) and `insufficient_credits` reaches it
// as 5 — rather than both surfacing as a stream that carried no events.
func decodeSceneEditRefusal(resp *http.Response) error {
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("scene edit: backend returned %s, not an event stream", resp.Header.Get("Content-Type"))
	}
	if body.Code == 0 || body.Code == 200 {
		return fmt.Errorf("scene edit: backend accepted the request but sent no event stream")
	}
	code, ok := httpclient.MapBusinessCode(body.Code)
	if !ok {
		code = "business_error"
	}
	return &errs.Object{
		SchemaVersion: "1",
		Code:          code,
		Message:       strings.TrimSpace(body.Message),
		TraceID:       body.TraceID,
		Retryable:     httpclient.IsRetryableCode(code),
	}
}

func sceneEditMessage(data json.RawMessage) string {
	var d sceneEditData
	if json.Unmarshal(data, &d) == nil {
		if msg := strings.TrimSpace(d.Msg); msg != "" {
			return msg
		}
		if msg := logMessage(d); msg != "" {
			return msg
		}
	}
	return "scene edit failed"
}
