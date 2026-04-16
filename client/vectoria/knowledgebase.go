package vectoria

import (
	"context"
	"fmt"
	"io"
)

type Document struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (c *Client) CreateKB(ctx context.Context, name string) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.http.Do(ctx, "POST", "/v1/knowledgebases", map[string]string{"name": name}, &resp); err != nil {
		return "", fmt.Errorf("create knowledgebase: %w", err)
	}
	return resp.ID, nil
}

func (c *Client) UploadDoc(ctx context.Context, kbID, fileName string, file io.Reader) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/v1/knowledgebases/%s/documents/file", kbID)
	if err := c.http.DoUpload(ctx, path, "file", fileName, file, &doc); err != nil {
		return nil, fmt.Errorf("upload document: %w", err)
	}
	return &doc, nil
}

func (c *Client) UploadURL(ctx context.Context, kbID, url string) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/v1/knowledgebases/%s/documents/url", kbID)
	if err := c.http.Do(ctx, "POST", path, map[string]string{"url": url}, &doc); err != nil {
		return nil, fmt.Errorf("upload URL: %w", err)
	}
	return &doc, nil
}

func (c *Client) GetDocStatus(ctx context.Context, kbID, docID string) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/v1/knowledgebases/%s/documents/%s", kbID, docID)
	if err := c.http.Do(ctx, "GET", path, nil, &doc); err != nil {
		return nil, fmt.Errorf("get document status: %w", err)
	}
	return &doc, nil
}

func (c *Client) DeleteDoc(ctx context.Context, kbID, docID string) error {
	path := fmt.Sprintf("/v1/knowledgebases/%s/documents/%s", kbID, docID)
	if err := c.http.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}
