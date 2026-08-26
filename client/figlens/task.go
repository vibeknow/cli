package figlens

import (
	"context"
	"fmt"
)

// Backend video_kind wire values. The CLI flag names (`replica`, `image`,
// `handdraw`) map to these via cmd.resolveMode; the wire values are not
// CLI-facing names.
//
// VideoKindScriptLock is deliberately absent from the generation path:
// 原稿锁定 is no longer a video_kind, it is the orthogonal `script_lock`
// boolean below (it composes with the standard line and with image2 alike).
// The one place the backend still keys on the string is the prompt-optimize
// endpoint, where `video_kind: "script_lock"` selects a fixed prompt for
// display; see OptimizeVideoKindScriptLock in optimize.go.
const (
	VideoKindReplica  = "replica"
	VideoKindImage2   = "image2"
	VideoKindHandDraw = "hand-draw"
)

type Task struct {
	TaskID    int64  `json:"task_id"`
	SessionID string `json:"session_id"`
	WorkID    int64  `json:"work_id"`
	V         int    `json:"v,omitempty"`
}

type InitTaskParams struct {
	Engine      Engine `json:"-"` // selects wire v field, never emitted as a body key
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VideoKind   string `json:"video_kind,omitempty"`
	// ScriptLock is 原稿锁定: use the document verbatim as the narration
	// script instead of writing one. Orthogonal to VideoKind — it composes
	// with the standard line and with image2. Sending it is what triggers
	// the backend's script-quality preflight (length, character set, LLM
	// suitability judgement), which rejects unusable scripts here rather
	// than after a full billed pipeline run; the preflight keys on this
	// field alone, so it must travel with the knowledge_id/doc_id pair.
	ScriptLock bool `json:"script_lock,omitempty"`
	// SelectedImageIndexes are mandatory-image picks from `vk doc images`
	// (user_clip image_index values). Backend validates ownership, promotes
	// the draft clips to the task, and snapshots them onto the new work.
	// Only honored on the pipeline standard line; rejected for replica.
	SelectedImageIndexes []int `json:"selected_image_indexes,omitempty"`
	// PageCount is image-mode-only and must match what the stream request
	// will send: init runs the image2 feasibility preflight (word count ≥
	// pages × 50, mandatory-image count ≤ pages) against this value,
	// defaulting to 4 when omitted — so omitting it here while streaming a
	// real page count preflights a different request than the one that runs.
	PageCount int `json:"page_count,omitempty"`
}

type initTaskWire struct {
	V int `json:"v"`
	InitTaskParams
}

func (c *Client) InitTask(ctx context.Context, p InitTaskParams) (*Task, error) {
	var t Task
	body := initTaskWire{V: p.Engine.Wire(), InitTaskParams: p}
	if err := c.http.Do(ctx, "POST", "/v1/tasks/init", body, &t); err != nil {
		return nil, fmt.Errorf("init task: %w", err)
	}
	return &t, nil
}
