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
