package figlens

import (
	"context"
	"fmt"
)

// ExtractedImage is one candidate document image returned by
// /v1/task/extractDocImages. ImageIndex is the stable per-(user, doc)
// identifier users pass back via `vk create --images`.
type ExtractedImage struct {
	ImageIndex  int    `json:"image_index"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Context     string `json:"context,omitempty"`
}

type extractDocImagesRequest struct {
	KnowledgeID string `json:"knowledge_id"`
	DocID       string `json:"doc_id"`
	VideoKind   string `json:"video_kind,omitempty"`
}

type extractDocImagesResponse struct {
	Images []ExtractedImage `json:"images"`
}

// ExtractDocImages pulls candidate images out of a parsed knowledge
// document so the user can pick mandatory ones. Idempotent on the backend:
// repeated calls for the same (user, doc) reuse the existing draft clips.
func (c *Client) ExtractDocImages(ctx context.Context, knowledgeID, docID, videoKind string) ([]ExtractedImage, error) {
	var resp extractDocImagesResponse
	body := extractDocImagesRequest{KnowledgeID: knowledgeID, DocID: docID, VideoKind: videoKind}
	if err := c.http.Do(ctx, "POST", "/v1/task/extractDocImages", body, &resp); err != nil {
		return nil, fmt.Errorf("extract doc images: %w", err)
	}
	return resp.Images, nil
}
