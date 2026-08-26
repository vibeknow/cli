package stage_test

import (
	"testing"

	"github.com/vibeknow/cli/internal/stage"
)

// The mapping mirrors go-figlens's nodeEventI18nKeys registry: exactly the
// step_ids the backend actually emits, nothing else. A node in this table
// that the backend never sends is a lie; a node the backend sends that is
// missing here silently loses its stage attribution.
func TestNodeToStage(t *testing.T) {
	tests := []struct {
		node string
		want string
	}{
		// standard line
		{"big_director", "outline"},
		{"script_writing", "outline"},
		{"storyboard_plan", "outline"},
		{"tts_generate", "tts"},
		{"scene_filling", "render"},
		{"bg_images", "render"},
		{"cover", "render"},
		{"bgm", "render"},
		{"video_package", "publish"},
		// replica line
		{"doc_dissect", "outline"},
		{"doc_replica_plan", "outline"},
		{"doc_replica_shoot", "render"},
		// image line
		{"image2_theme_select", "outline"},
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

// Step_ids the backend never emits must stay out of the table. Each entry
// names why it is absent so a future re-add is a deliberate decision:
//   - prepare/knowledge_detail/theme_select/avatar_submit/suggest/video_finish:
//     graph nodes with no i18n registration — no process events on the wire.
//   - text_speech/content_analyze/video_director/design/scene_generate:
//     pre-rework names, gone from the backend graph entirely.
//   - image2_style_select: never existed on the wire (the real step_id is
//     image2_theme_select).
//   - handdraw_*: the hand-draw line has no i18n registration at all; the
//     backend is silent for its whole middle section.
func TestNodeToStage_UnmappedNodes(t *testing.T) {
	for _, node := range []string{
		"prepare", "knowledge_detail", "theme_select", "avatar_submit",
		"suggest", "video_finish",
		"text_speech", "content_analyze", "video_director", "design", "scene_generate",
		"image2_style_select",
		"handdraw_material_reconstruct", "handdraw_theme_select",
		"handdraw_storyboard", "handdraw_gen", "handdraw_vectorize",
		"nonexistent_node", "",
	} {
		if _, ok := stage.FromNode(node); ok {
			t.Errorf("FromNode(%q) unexpectedly mapped; must fall through to free-form progress", node)
		}
	}
}

func TestStageOrder(t *testing.T) {
	stages := stage.OrderedStages()
	// parse and suggest are gone: the backend never emits events for the
	// nodes that used to feed them, so a six-stage bar showed two segments
	// that could not light up.
	expected := []string{"outline", "tts", "render", "publish"}
	if len(stages) != len(expected) {
		t.Fatalf("expected %d stages, got %d: %v", len(expected), len(stages), stages)
	}
	for i, s := range stages {
		if s != expected[i] {
			t.Fatalf("stage[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestAllNodes(t *testing.T) {
	nodes := stage.AllNodes()
	if len(nodes) != 9 {
		t.Fatalf("expected 9 standard-line nodes, got %d: %v", len(nodes), nodes)
	}
	for _, n := range nodes {
		if !stage.IsKnownNode(n) {
			t.Errorf("ordered node %q missing from nodeToStage", n)
		}
	}
}

// The image-mode wire names carry an internal model codename; DisplayName
// must sanitize them and pass everything else through untouched.
func TestDisplayName(t *testing.T) {
	tests := []struct{ node, want string }{
		{"image2_theme_select", "style_select"},
		{"image2_storyboard", "image_storyboard"},
		{"image2_gen", "image_gen"},
		{"script_writing", "script_writing"},
		{"doc_replica_shoot", "doc_replica_shoot"},
	}
	for _, tt := range tests {
		if got := stage.DisplayName(tt.node); got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.node, got, tt.want)
		}
	}
}
