package stage_test

import (
	"testing"

	"github.com/vibeknow/cli/internal/stage"
)

func TestNodeToStage(t *testing.T) {
	tests := []struct {
		node string
		want string
	}{
		{"prepare", "parse"},
		{"knowledge_detail", "parse"},
		{"text_speech", "outline"},
		{"content_analyze", "outline"},
		{"theme_select", "outline"},
		{"design", "outline"},
		{"tts_generate", "tts"},
		{"scene_generate", "render"},
		{"bg_images", "render"},
		{"cover", "render"},
		{"bgm", "render"},
		{"video_package", "publish"},
		{"video_finish", "publish"},
		{"suggest", "suggest"},
		{"doc_replica_plan", "outline"},
		{"doc_replica_shoot", "render"},
	}
	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			got, ok := stage.FromNode(tt.node)
			if !ok {
				t.Fatalf("node %q not found in mapping", tt.node)
			}
			if got != tt.want {
				t.Fatalf("FromNode(%q) = %q, want %q", tt.node, got, tt.want)
			}
		})
	}
}

func TestNodeToStage_Unknown(t *testing.T) {
	_, ok := stage.FromNode("nonexistent_node")
	if ok {
		t.Fatal("expected unknown node to return ok=false")
	}
}

func TestAllNodes(t *testing.T) {
	nodes := stage.AllNodes()
	if len(nodes) != 18 {
		t.Fatalf("expected 18 nodes, got %d", len(nodes))
	}
}

func TestStageOrder(t *testing.T) {
	stages := stage.OrderedStages()
	expected := []string{"parse", "outline", "tts", "render", "publish", "suggest"}
	if len(stages) != len(expected) {
		t.Fatalf("expected %d stages, got %d", len(expected), len(stages))
	}
	for i, s := range stages {
		if s != expected[i] {
			t.Fatalf("stage[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

// Nodes introduced by the go-figlens pipeline rework (script_writing /
// video_director / storyboard_plan / scene_filling replaced the old
// text_speech / content_analyze / design / scene_generate naming; the old
// IDs stay mapped for older deployments).
func TestNodeToStage_ReworkedPipelineNodes(t *testing.T) {
	tests := []struct {
		node string
		want string
	}{
		{"script_writing", "outline"},
		{"video_director", "outline"},
		{"storyboard_plan", "outline"},
		{"scene_filling", "render"},
	}
	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			got, ok := stage.FromNode(tt.node)
			if !ok {
				t.Fatalf("node %q not found in mapping", tt.node)
			}
			if got != tt.want {
				t.Fatalf("FromNode(%q) = %q, want %q", tt.node, got, tt.want)
			}
		})
	}
}

// Wire step_ids from the image-mode (讲稿生图) line and the replica
// doc_dissect split. The image2_* prefix is the backend's wire naming;
// DisplayName sanitizes it before any user-facing output.
func TestNodeToStage_ImageModeAndDissectNodes(t *testing.T) {
	tests := []struct {
		node string
		want string
	}{
		{"doc_dissect", "outline"},
		{"image2_style_select", "outline"},
		{"image2_storyboard", "outline"},
		{"image2_gen", "render"},
	}
	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			got, ok := stage.FromNode(tt.node)
			if !ok {
				t.Fatalf("node %q not found in mapping", tt.node)
			}
			if got != tt.want {
				t.Fatalf("FromNode(%q) = %q, want %q", tt.node, got, tt.want)
			}
		})
	}
}

// DisplayName must sanitize wire step_ids that carry internal codenames
// and pass everything else through unchanged.
func TestDisplayName_SanitizesCodenames(t *testing.T) {
	tests := map[string]string{
		"image2_style_select": "style_select",
		"image2_storyboard":   "image_storyboard",
		"image2_gen":          "image_gen",
		"prepare":             "prepare",     // identity fallback
		"future_node":         "future_node", // unknown passes through
	}
	for node, want := range tests {
		if got := stage.DisplayName(node); got != want {
			t.Fatalf("DisplayName(%q) = %q, want %q", node, got, want)
		}
	}
}
