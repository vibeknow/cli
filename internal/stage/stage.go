// Package stage maps figlens pipeline node names to logical CLI stages.
// See spec §5.3: pipeline nodes → 6 stages (parse/outline/tts/render/publish/suggest).
// The backend's pipeline rework renamed the middle of the graph
// (script_writing/video_director/storyboard_plan/scene_filling replaced
// text_speech/content_analyze/design/scene_generate); both generations stay
// mapped so the CLI shows progress against old and new deployments alike.
package stage

var nodeToStage = map[string]string{
	"prepare":          "parse",
	"knowledge_detail": "parse",
	"text_speech":      "outline",
	"content_analyze":  "outline",
	"script_writing":   "outline",
	"video_director":   "outline",
	"theme_select":     "outline",
	"storyboard_plan":  "outline",
	"design":           "outline",
	"tts_generate":     "tts",
	"scene_generate":   "render",
	"scene_filling":    "render",
	"bg_images":        "render",
	"cover":            "render",
	"bgm":              "render",
	"video_package":    "publish",
	"video_finish":     "publish",
	"suggest":          "suggest",
	"doc_dissect":      "outline",
	"doc_replica_plan": "outline",
	"doc_replica_shoot": "render",
	"image2_style_select": "outline",
	"image2_storyboard":   "outline",
	"image2_gen":          "render",
}

// nodeDisplayName holds only the step_ids whose display name differs from
// the wire value — DisplayName falls back to the raw name for the rest.
// The image-mode step_ids carry an internal model codename on the wire;
// display sanitized names so it never reaches user output.
var nodeDisplayName = map[string]string{
	"image2_style_select": "style_select",
	"image2_storyboard":   "image_storyboard",
	"image2_gen":          "image_gen",
}

var orderedStages = []string{"parse", "outline", "tts", "render", "publish", "suggest"}

var orderedNodes = []string{
	"prepare", "knowledge_detail",
	"text_speech", "content_analyze", "script_writing", "video_director",
	"theme_select", "storyboard_plan", "design",
	"tts_generate",
	"scene_generate", "scene_filling", "bg_images", "cover", "bgm",
	"video_package", "video_finish",
	"suggest",
}

func FromNode(node string) (string, bool) {
	s, ok := nodeToStage[node]
	return s, ok
}

// DisplayName returns a human-readable label for a pipeline node.
func DisplayName(node string) string {
	if name, ok := nodeDisplayName[node]; ok {
		return name
	}
	return node
}

// IsKnownNode reports whether the node is a recognised pipeline step.
func IsKnownNode(node string) bool {
	_, ok := nodeToStage[node]
	return ok
}

func AllNodes() []string {
	out := make([]string, len(orderedNodes))
	copy(out, orderedNodes)
	return out
}

func OrderedStages() []string {
	out := make([]string, len(orderedStages))
	copy(out, orderedStages)
	return out
}
