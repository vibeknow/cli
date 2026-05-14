// Package stage maps figlens pipeline node names to logical CLI stages.
// See spec §5.3: 14 nodes → 6 stages (parse/outline/tts/render/publish/suggest).
package stage

var nodeToStage = map[string]string{
	"prepare":          "parse",
	"knowledge_detail": "parse",
	"text_speech":      "outline",
	"content_analyze":  "outline",
	"theme_select":     "outline",
	"design":           "outline",
	"tts_generate":     "tts",
	"scene_generate":   "render",
	"bg_images":        "render",
	"cover":            "render",
	"bgm":              "render",
	"video_package":    "publish",
	"video_finish":     "publish",
	"suggest":          "suggest",
	"doc_replica_plan": "outline",
	"doc_replica_shoot": "render",
}

var nodeDisplayName = map[string]string{
	"prepare":          "prepare",
	"knowledge_detail": "knowledge_detail",
	"text_speech":      "text_speech",
	"content_analyze":  "content_analyze",
	"theme_select":     "theme_select",
	"design":           "design",
	"tts_generate":     "tts_generate",
	"scene_generate":   "scene_generate",
	"bg_images":        "bg_images",
	"cover":            "cover",
	"bgm":              "bgm",
	"video_package":    "video_package",
	"video_finish":     "video_finish",
	"suggest":          "suggest",
	"doc_replica_plan": "doc_replica_plan",
	"doc_replica_shoot": "doc_replica_shoot",
}

var orderedStages = []string{"parse", "outline", "tts", "render", "publish", "suggest"}

var orderedNodes = []string{
	"prepare", "knowledge_detail",
	"text_speech", "content_analyze", "theme_select", "design",
	"tts_generate",
	"scene_generate", "bg_images", "cover", "bgm",
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
