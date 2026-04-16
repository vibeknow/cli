package sse_test

import (
	"strings"
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/internal/sse"
)

func TestReader_BasicEvents(t *testing.T) {
	input := "data: {\"type\":\"process\"}\n\ndata: {\"type\":\"done\"}\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != `{"type":"process"}` {
		t.Fatalf("got data %q, want process event", ev.Data)
	}

	ev, err = r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != `{"type":"done"}` {
		t.Fatalf("got data %q, want done event", ev.Data)
	}
}

func TestReader_MultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "line1\nline2" {
		t.Fatalf("got data %q, want multi-line concat", ev.Data)
	}
}

func TestReader_EventField(t *testing.T) {
	input := "event: error\ndata: {\"msg\":\"fail\"}\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "error" {
		t.Fatalf("got event %q, want error", ev.Event)
	}
}

func TestReader_IDField(t *testing.T) {
	input := "id: 42\ndata: hello\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "42" {
		t.Fatalf("got id %q, want 42", ev.ID)
	}
}

func TestReader_SkipsComments(t *testing.T) {
	input := ": keep-alive\ndata: real\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "real" {
		t.Fatalf("got data %q, want real", ev.Data)
	}
}

func TestReader_EOF(t *testing.T) {
	input := "data: last\n\n"
	r := sse.NewReader(strings.NewReader(input))

	_, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error on first: %v", err)
	}

	_, err = r.Next()
	if err == nil {
		t.Fatal("expected io.EOF, got nil")
	}
}
