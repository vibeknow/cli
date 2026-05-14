package figlens

import (
	"context"
	"fmt"
)

// Backend video_kind wire values. The CLI flag names (`replica`, `script`)
// map to these via cmd.resolveVideoKind.
const (
	VideoKindReplica    = "replica"
	VideoKindScriptLock = "script_lock"
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
