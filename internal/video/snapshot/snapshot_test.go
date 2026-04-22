package snapshot_test

import (
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

func TestShareURL(t *testing.T) {
	cases := []struct {
		name, base, token, want string
	}{
		{"default base", "", "abc123", "https://vibeknow.com/share/abc123"},
		{"custom base", "https://self.example/s", "abc123", "https://self.example/s/abc123"},
		{"trailing slash stripped", "https://self.example/s/", "abc123", "https://self.example/s/abc123"},
		{"empty token returns empty", "https://self.example/s", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := snapshot.ShareURL(tc.base, tc.token)
			if got != tc.want {
				t.Fatalf("ShareURL(%q, %q) = %q, want %q", tc.base, tc.token, got, tc.want)
			}
		})
	}
}

func TestBuild_PreviewReadyExportIdle(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:    42,
		SessionID: "s_1",
		Work: &figlens.Work{
			ID: 99, Title: "Hi", Duration: 30_000,
			HtmlPath: "w/index.html", ShareToken: "tok",
			Exporting: 0, VideoPath: "",
		},
		ShareBase: "https://vibeknow.com/share",
	})
	if !s.Preview.Ready {
		t.Fatal("preview should be ready")
	}
	if s.Preview.ShareURL != "https://vibeknow.com/share/tok" {
		t.Fatalf("share_url = %q", s.Preview.ShareURL)
	}
	if s.Export.Status != snapshot.StatusIdle {
		t.Fatalf("export.status = %q", s.Export.Status)
	}
	if len(s.NextActions) == 0 || !containsCmd(s.NextActions, "vk video export 42") {
		t.Fatalf("expected export next_action, got %+v", s.NextActions)
	}
}

func TestBuild_ExportRunningFromExportingFlag(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:    42,
		SessionID: "s_1",
		Work:      &figlens.Work{ShareToken: "t", Exporting: 1},
		ShareBase: "https://vibeknow.com/share",
	})
	if s.Export.Status != snapshot.StatusRunning {
		t.Fatalf("export.status = %q, want running", s.Export.Status)
	}
	if !containsCmd(s.NextActions, "vk video status 42") {
		t.Fatalf("expected poll next_action, got %+v", s.NextActions)
	}
}

func TestBuild_ExportSucceeded_VideoPathPresent(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:    42,
		SessionID: "s_1",
		Work:      &figlens.Work{ShareToken: "t", VideoPath: "v/out.mp4"},
		ShareBase: "https://vibeknow.com/share",
	})
	if s.Export.Status != snapshot.StatusSucceeded {
		t.Fatalf("export.status = %q", s.Export.Status)
	}
	if s.Export.VideoPath != "v/out.mp4" {
		t.Fatalf("video_path = %q", s.Export.VideoPath)
	}
	if !containsCmd(s.NextActions, "vk video download 42") {
		t.Fatalf("expected download next_action, got %+v", s.NextActions)
	}
}

func TestBuild_ExportFailed_FromExportResult(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:    42,
		SessionID: "s_1",
		Work:      &figlens.Work{ShareToken: "t"},
		Export:    &figlens.ExportResult{Status: "failed", Error: "boom"},
		ShareBase: "https://vibeknow.com/share",
	})
	if s.Export.Status != snapshot.StatusFailed {
		t.Fatalf("export.status = %q", s.Export.Status)
	}
	if s.Export.Error != "boom" {
		t.Fatalf("error = %q", s.Export.Error)
	}
	if !containsCmd(s.NextActions, "vk video export 42") {
		t.Fatalf("expected export-retry next_action, got %+v", s.NextActions)
	}
}

func TestBuild_PreviewNotReady(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:    42,
		SessionID: "s_1",
		Work:      &figlens.Work{ShareToken: ""},
		ShareBase: "https://vibeknow.com/share",
	})
	if s.Preview.Ready {
		t.Fatal("preview should not be ready without share_token")
	}
	if !containsCmd(s.NextActions, "vk video wait 42") {
		t.Fatalf("expected wait next_action, got %+v", s.NextActions)
	}
}

func TestBuild_ExportRunningFromExportResult_PopulatesProgress(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID:       42,
		SessionID:    "s_1",
		Work:         &figlens.Work{ShareToken: "t"},
		Export:       &figlens.ExportResult{Status: "processing", Progress: 47, ProgressMsg: "rendering"},
		ExportTaskID: "exp_7",
		ShareBase:    "https://vibeknow.com/share",
	})
	if s.Export.Status != snapshot.StatusRunning {
		t.Fatalf("export.status = %q", s.Export.Status)
	}
	if s.Export.Progress != 47 || s.Export.ProgressMsg != "rendering" {
		t.Fatalf("progress fields = %+v", s.Export)
	}
	if s.Export.ExportTaskID != "exp_7" {
		t.Fatalf("export_task_id = %q", s.Export.ExportTaskID)
	}
}

func TestBuild_NoNextActionsWhenTaskIDZero(t *testing.T) {
	s := snapshot.Build(snapshot.BuildInput{
		TaskID: 0, SessionID: "s_1",
		Work: &figlens.Work{ShareToken: "t"},
	})
	if len(s.NextActions) != 0 {
		t.Fatalf("expected no next_actions when TaskID=0, got %+v", s.NextActions)
	}
}

func containsCmd(actions []snapshot.Action, substr string) bool {
	for _, a := range actions {
		if strings.Contains(a.Command, substr) {
			return true
		}
	}
	return false
}
