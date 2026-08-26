// Package stage maps figlens pipeline node names to logical CLI stages.
//
// The backend does not emit stages — it emits `log.step_id` values, which
// are graph node names, and only for nodes registered in its i18n table
// (go-figlens internal/pipeline/node/base.go, nodeEventI18nKeys). Nodes
// outside that table — prepare, knowledge_detail, theme_select,
// avatar_submit, suggest, video_finish, and the whole handdraw_* line —
// never produce a process event, so they must not appear here: a mapping
// for a node that never arrives is documentation telling lies.
//
// Four stages can actually light up on the v=3 pipeline:
// outline → tts → render → publish. The v=2 agent engine emits no
// step_ids at all (free-form progress only). Two step_ids arrive outside
// this table by design and are handled upstream in client/figlens:
// the "" (empty) step_id used by the run-started event and by stalled-run
// pending heartbeats, and any future node this build has never heard of —
// both are forwarded as free-form progress rather than dropped.
package stage

var nodeToStage = map[string]string{
	// Standard line (灵活创作 / 原稿锁定).
	"big_director":    "outline", // pre-script planning, the longest LLM node (1–2 min)
	"script_writing":  "outline",
	"storyboard_plan": "outline",
	"tts_generate":    "tts",
	"scene_filling":   "render",
	"bg_images":       "render",
	"cover":           "render",
	"bgm":             "render",
	"video_package":   "publish",

	// Replica line (PPT 讲解). doc_replica_plan registers empty lifecycle
	// text and emits its own staged progress messages from inside the node;
	// doc_replica_shoot events are likewise hand-emitted. Both still carry
	// their step_id, so they map here.
	"doc_dissect":       "outline",
	"doc_replica_plan":  "outline",
	"doc_replica_shoot": "render",

	// Image line (图解视频). The image2_* prefix is the backend's wire
	// naming; DisplayName sanitizes it before any user-facing output.
	"image2_theme_select": "outline",
	"image2_storyboard":   "outline",
	"image2_gen":          "render",
}

// nodeDisplayName holds only the step_ids whose display name differs from
// the wire value — DisplayName falls back to the raw name for the rest.
// The image-mode step_ids carry an internal model codename on the wire;
// display sanitized names so it never reaches user output.
var nodeDisplayName = map[string]string{
	"image2_theme_select": "style_select",
	"image2_storyboard":   "image_storyboard",
	"image2_gen":          "image_gen",
}

var orderedStages = []string{"outline", "tts", "render", "publish"}

// orderedNodes lists the emitting nodes of the standard line in graph
// order. Mode-specific nodes (doc_*, image2_*) branch off this spine and
// are not part of the canonical ordering.
var orderedNodes = []string{
	"big_director", "script_writing", "storyboard_plan",
	"tts_generate",
	"scene_filling", "bg_images", "cover", "bgm",
	"video_package",
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
