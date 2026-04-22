package figlens

import (
	"context"
	"fmt"
)

type ExportResult struct {
	Status      string `json:"status"`
	Progress    int    `json:"progress,omitempty"`
	ProgressMsg string `json:"progress_msg,omitempty"`
	VideoPath   string `json:"video_path,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (c *Client) ExportVideo(ctx context.Context, sessionID string) (int64, error) {
	var resp struct {
		TaskID int64 `json:"task_id"`
	}
	if err := c.http.Do(ctx, "POST", "/v1/agent2forVideo/exportRemoteV2",
		map[string]string{"session_id": sessionID}, &resp); err != nil {
		return 0, fmt.Errorf("export video: %w", err)
	}
	return resp.TaskID, nil
}

func (c *Client) GetExportResult(ctx context.Context, exportTaskID int64) (*ExportResult, error) {
	var r ExportResult
	if err := c.http.Do(ctx, "POST", "/v1/agent2forVideo/exportResultV2",
		map[string]int64{"task_id": exportTaskID}, &r); err != nil {
		return nil, fmt.Errorf("get export result: %w", err)
	}
	return &r, nil
}

func (c *Client) SignedURL(ctx context.Context, objectKey string) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.http.Do(ctx, "POST", "/v1/agent2forVideo/signedUrl",
		map[string]string{"object_key": objectKey}, &resp); err != nil {
		return "", fmt.Errorf("signed url: %w", err)
	}
	return resp.URL, nil
}
