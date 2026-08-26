package figlens_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/errs"
)

func editServer(t *testing.T, handler http.HandlerFunc) *figlens.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return figlens.New(srv.URL, staticToken("tok"))
}

func sseFrame(data string) string {
	return fmt.Sprintf("event: data\ndata: {\"code\":200,\"data\":%s}\n\n", data)
}

func TestStreamSceneEdit_ReportsProgressAndResult(t *testing.T) {
	c := editServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scene/edit" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseFrame(`{"type":"edit_start","log":{"message":"开始处理","status":"start"}}`))
		// Comment-only heartbeats sit between real frames on a slow edit and
		// must not be mistaken for events.
		fmt.Fprint(w, ": heartbeat\n\n")
		fmt.Fprint(w, sseFrame(`{"type":"process","log":{"message":"配音已重新生成","status":"success"}}`))
		fmt.Fprint(w, sseFrame(`{"type":"edit_completed","data":{"html_path":"https://cdn.test/p.html"}}`))
	})

	var got []figlens.SceneEditEvent
	err := c.StreamSceneEdit(context.Background(), figlens.SceneEditParams{SessionID: "s"}, func(ev figlens.SceneEditEvent) {
		got = append(got, ev)
	})
	if err != nil {
		t.Fatalf("StreamSceneEdit: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3: %+v", len(got), got)
	}
	if got[0].Type != "edit.started" {
		t.Errorf("first event = %+v", got[0])
	}
	if got[1].Type != "edit.progress" || got[1].Status != "success" || got[1].Message != "配音已重新生成" {
		t.Errorf("progress event = %+v", got[1])
	}
	if got[2].Type != "edit.succeeded" || got[2].PreviewURL != "https://cdn.test/p.html" {
		t.Errorf("terminal event = %+v", got[2])
	}
}

// TestStreamSceneEdit_MidStreamFailureIsAnError covers the shape that makes
// this stream different: a pipeline failure arrives inside a *code 200*
// envelope, as ChatReplyContent with msg_type ERROR. Read off the envelope
// code it looks like a successful stream that simply never completed, and
// the caller would report an edit that did not happen.
func TestStreamSceneEdit_MidStreamFailureIsAnError(t *testing.T) {
	c := editServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseFrame(`{"type":"edit_start","log":{"message":"开始处理","status":"start"}}`))
		fmt.Fprint(w, sseFrame(`{"msg_type":"ERROR","msg":"重新生成配音失败"}`))
	})

	var got []figlens.SceneEditEvent
	err := c.StreamSceneEdit(context.Background(), figlens.SceneEditParams{SessionID: "s"}, func(ev figlens.SceneEditEvent) {
		got = append(got, ev)
	})
	if err == nil {
		t.Fatal("a failed edit returned no error")
	}
	if !strings.Contains(err.Error(), "重新生成配音失败") {
		t.Errorf("error does not carry the backend's reason: %v", err)
	}
	for _, ev := range got {
		if ev.Type == "edit.succeeded" {
			t.Error("a failed edit emitted edit.succeeded")
		}
	}
}

// TestStreamSceneEdit_PreStreamRefusalKeepsItsCode covers the lock and the
// credit precheck, both answered before the SSE headers go out — as JSON, on
// HTTP 200, because the gateway maps unrecognised business codes that way.
func TestStreamSceneEdit_PreStreamRefusalKeepsItsCode(t *testing.T) {
	cases := []struct {
		name      string
		code      int
		wantCode  string
		retryable bool
	}{
		{"edit lock held", 100008, "work_edit_busy", true},
		{"no credits", 100001, "insufficient_credits", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := editServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": tc.code, "message": "nope"})
			})
			err := c.StreamSceneEdit(context.Background(), figlens.SceneEditParams{SessionID: "s"}, func(figlens.SceneEditEvent) {
				t.Error("a refused edit emitted an event")
			})
			if err == nil {
				t.Fatal("refusal returned no error")
			}
			var o *errs.Object
			if !asObject(err, &o) {
				t.Fatalf("error is not an errs.Object, so it never reaches the exit-code table: %T %v", err, err)
			}
			if o.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", o.Code, tc.wantCode)
			}
			if o.Retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v", o.Retryable, tc.retryable)
			}
		})
	}
}

// TestStreamSceneEdit_TerminalEventSurvivesALargePayload guards the size
// limit that is easy to miss because it does not look like a size problem.
//
// edit_completed carries the whole rendered package — every scene's layout
// code — on one `data:` line. Past bufio.Scanner's 64KB default the scan
// stops, the stream ends, and the run reads as "the backend went quiet" on
// the one event that says the work is finished.
func TestStreamSceneEdit_TerminalEventSurvivesALargePayload(t *testing.T) {
	bulk := strings.Repeat("x", 300_000)
	c := editServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		payload, _ := json.Marshal(map[string]any{
			"type": "edit_completed",
			"data": map[string]any{"html_path": "https://cdn.test/p.html", "data": map[string]any{"code": bulk}},
		})
		fmt.Fprintf(w, "event: data\ndata: {\"code\":200,\"data\":%s}\n\n", payload)
	})

	var url string
	err := c.StreamSceneEdit(context.Background(), figlens.SceneEditParams{SessionID: "s"}, func(ev figlens.SceneEditEvent) {
		if ev.PreviewURL != "" {
			url = ev.PreviewURL
		}
	})
	if err != nil {
		t.Fatalf("StreamSceneEdit: %v", err)
	}
	if url != "https://cdn.test/p.html" {
		t.Errorf("preview URL lost on a large terminal event: %q", url)
	}
}

// asObject is errors.As with the concrete type spelled out, so the assertion
// above reads as what it checks rather than as reflection plumbing.
func asObject(err error, target **errs.Object) bool {
	return errors.As(err, target)
}
