package stage_test

import (
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/internal/stage"
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
	if len(nodes) != 14 {
		t.Fatalf("expected 14 nodes, got %d", len(nodes))
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
