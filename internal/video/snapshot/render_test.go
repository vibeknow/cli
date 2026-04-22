package snapshot_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

func TestRenderText_PreviewReady(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID: 42, SessionID: "s_1",
		Work: &figlens.Work{
			ID: 99, Title: "Hello", Duration: 42000, ShareToken: "tok",
		},
		ShareBase: "https://vibeknow.com/share",
	})
	var out, errOut bytes.Buffer
	snapshot.RenderText(&out, &errOut, s)

	stdout := out.String()
	for _, want := range []string{
		"task_id=42", "session_id=s_1", "work_id=99",
		"title=Hello", "duration=42s",
		"share_url=https://vibeknow.com/share/tok",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q\n%s", want, stdout)
		}
	}
	if !strings.Contains(errOut.String(), "vk video export 42") {
		t.Fatalf("stderr missing export hint:\n%s", errOut.String())
	}
}

func TestRenderNDJSON_EmitsTerminalSnapshot(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID: 42, SessionID: "s",
		Work: &figlens.Work{ID: 9, ShareToken: "t"},
	})
	var buf bytes.Buffer
	if err := snapshot.RenderNDJSON(&buf, s); err != nil {
		t.Fatal(err)
	}
	// Parse the single line.
	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if decoded["type"] != "snapshot" {
		t.Fatalf("type = %v", decoded["type"])
	}
	if decoded["schema_version"] != "1" {
		t.Fatalf("schema_version = %v", decoded["schema_version"])
	}
	if _, ok := decoded["preview"]; !ok {
		t.Fatal("preview missing")
	}
}

func TestRenderJSON_HasSchemaVersion(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID: 1, SessionID: "s", Work: &figlens.Work{ShareToken: "t"},
	})
	var buf bytes.Buffer
	if err := snapshot.RenderJSON(&buf, s); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if decoded["schema_version"] != "1" {
		t.Fatalf("schema_version = %v", decoded["schema_version"])
	}
	if _, ok := decoded["preview"]; !ok {
		t.Fatal("preview missing")
	}
	if _, ok := decoded["next_actions"]; !ok {
		t.Fatal("next_actions missing")
	}
}
