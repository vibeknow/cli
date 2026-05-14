package figlens

import (
	"context"
	"fmt"
)

type Task struct {
	TaskID    int64  `json:"task_id"`
	SessionID string `json:"session_id"`
	WorkID    int64  `json:"work_id"`
	V         int    `json:"v,omitempty"`
}

type InitTaskParams struct {
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VideoKind   string `json:"video_kind,omitempty"`
}

type initTaskWire struct {
	V           int    `json:"v"`
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VideoKind   string `json:"video_kind,omitempty"`
}

func (c *Client) InitTask(ctx context.Context, p InitTaskParams) (*Task, error) {
	var t Task
	body := initTaskWire{
		V:           3,
		KnowledgeID: p.KnowledgeID,
		DocID:       p.DocID,
		VideoKind:   p.VideoKind,
	}
	if err := c.http.Do(ctx, "POST", "/v1/tasks/init", body, &t); err != nil {
		return nil, fmt.Errorf("init task: %w", err)
	}
	return &t, nil
}
