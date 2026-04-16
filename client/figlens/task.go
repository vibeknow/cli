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

func (c *Client) InitTask(ctx context.Context) (*Task, error) {
	var t Task
	if err := c.http.Do(ctx, "POST", "/v1/tasks/init", map[string]int{"v": 3}, &t); err != nil {
		return nil, fmt.Errorf("init task: %w", err)
	}
	return &t, nil
}
