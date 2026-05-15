package figlens_test

import (
	"reflect"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestNDJSONFields_TaskSucceededOmitsAbsentFields(t *testing.T) {
	// Agent engine has no duration_ms — must be omitted, not zero-emitted,
	// so a consumer doing `if "duration_ms" in event` works.
	ev := figlens.StreamEvent{Type: "task.succeeded", SessionID: "s", VideoURL: "https://x"}
	got := ev.NDJSONFields()
	if _, has := got["duration_ms"]; has {
		t.Fatalf("duration_ms must be omitted when zero, got %v", got["duration_ms"])
	}
	if got["video_url"] != "https://x" {
		t.Fatalf("video_url = %v, want https://x", got["video_url"])
	}
}

func TestNDJSONFields_TaskSucceededIncludesPresentFields(t *testing.T) {
	ev := figlens.StreamEvent{
		Type: "task.succeeded", SessionID: "s",
		VideoURL: "https://x", DurationMs: 12345,
	}
	got := ev.NDJSONFields()
	want := map[string]any{
		"type":        "task.succeeded",
		"session_id":  "s",
		"video_url":   "https://x",
		"duration_ms": int64(12345),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNDJSONFields_TaskFailedAlwaysCarriesRetryable(t *testing.T) {
	// retryable must be present on every task.failed event — consumers
	// branch exit-code 4 vs 5 on it, so omission would silently change
	// behavior.
	ev := figlens.StreamEvent{Type: "task.failed", Code: "rate_limited", Message: "slow down", Retryable: true}
	got := ev.NDJSONFields()
	if got["retryable"] != true {
		t.Fatalf("retryable = %v, want true", got["retryable"])
	}
	if got["code"] != "rate_limited" {
		t.Fatalf("code = %v", got["code"])
	}
}

func TestNDJSONFields_NodeProgressUsesStatusNotStage(t *testing.T) {
	// node.progress is the agent-engine free-form event; it carries
	// `status` (start/success/error), not stage/node which are pipeline
	// concepts.
	ev := figlens.StreamEvent{Type: "node.progress", Status: "start", Message: "calling KB"}
	got := ev.NDJSONFields()
	if _, has := got["stage"]; has {
		t.Fatalf("node.progress must not carry stage/node, got %v", got)
	}
	if got["status"] != "start" {
		t.Fatalf("status = %v, want start", got["status"])
	}
}
