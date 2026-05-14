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

// DeleteKB removes a knowledgebase and all its documents.
// Used by `vk create` to clean up orphan kbs when upload-then-pipeline fails
// before the backend task takes ownership of the kb.
func (c *Client) DeleteKB(ctx context.Context, kbID string) error {
	path := fmt.Sprintf("/v1/knowledgebases/%s", kbID)
	if err := c.http.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("delete knowledgebase: %w", err)
	}
	return nil
}

type KB struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type ListKBsResponse struct {
	Total  int  `json:"total"`
	Offset int  `json:"offset"`
	Limit  int  `json:"limit"`
	Items  []KB `json:"items"`
}

// ListKBs paginates the user's knowledgebases.
// Backend caps limit at 100; callers walk pages by incrementing offset.
func (c *Client) ListKBs(ctx context.Context, offset, limit int) (*ListKBsResponse, error) {
	var resp ListKBsResponse
	path := fmt.Sprintf("/v1/knowledgebases?offset=%d&limit=%d", offset, limit)
	if err := c.http.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("list knowledgebases: %w", err)
	}
	return &resp, nil
}
