package figlens

import (
	"context"
	"fmt"
)

type Task struct {
	TaskID    int    `json:"task_id"`
	SessionID string `json:"session_id"`
	WorkID    string `json:"work_id"`
}

func (c *Client) InitTask(ctx context.Context) (*Task, error) {
	var t Task
	if err := c.do(ctx, "POST", "/v1/tasks/init", map[string]int{"v": 3}, &t); err != nil {
		return nil, fmt.Errorf("init task: %w", err)
	}
	return &t, nil
}
