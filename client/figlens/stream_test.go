package figlens_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestStreamChat_ProcessEvents(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"start","message":"Starting parse"}}}

data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"success","message":"Parse done"}}}

data: {"code":200,"data":{"type":"aim_result","session_id":"s_abc"}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{
		TaskID: 123, SessionID: "s_abc", Query: "test query",
		KnowledgeID: "kb_1", DocID: "doc_1", VoiceID: "v_1",
	}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected >= 3 events, got %d", len(events))
	}
	if events[0].Type != "node.started" || events[0].Node != "prepare" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Type != "node.succeeded" || events[1].Node != "prepare" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	if events[2].Type != "task.succeeded" || events[2].SessionID != "s_abc" {
		t.Fatalf("event[2] = %+v", events[2])
	}
}

func TestStreamChat_ErrorEvent(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"error","message":"pipeline failed"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{TaskID: 1, SessionID: "s_1", Query: "test"}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed event, got %v", events)
	}
}
