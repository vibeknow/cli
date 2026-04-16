package figlens

import (
	"context"
	"fmt"
)

type ExportResult struct {
	Status    string `json:"status"`
	VideoPath string `json:"video_path"`
}

func (c *Client) ExportVideo(ctx context.Context, sessionID string) (string, error) {
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/exportRemoteV2",
		map[string]string{"session_id": sessionID}, &resp); err != nil {
		return "", fmt.Errorf("export video: %w", err)
	}
	return resp.TaskID, nil
}

func (c *Client) GetExportResult(ctx context.Context, exportTaskID string) (*ExportResult, error) {
	var r ExportResult
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/exportResultV2",
		map[string]string{"task_id": exportTaskID}, &r); err != nil {
		return nil, fmt.Errorf("get export result: %w", err)
	}
	return &r, nil
}

func (c *Client) SignedURL(ctx context.Context, path string) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/signedUrl",
		map[string]string{"path": path}, &resp); err != nil {
		return "", fmt.Errorf("signed url: %w", err)
	}
	return resp.URL, nil
}
