package figlens

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/sse"
)

// GenerateCoverParams is the body of POST /v1/works/generateCoverStream.
type GenerateCoverParams struct {
	WorkID   int64  `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
}

// CoverResult is what a generated cover leaves behind.
type CoverResult struct {
	// CoverURL is the new current cover: a generated cover is applied as
	// soon as it is saved.
	CoverURL string `json:"cover_url"`
	// CoverURLs is every candidate on the work, the new one included.
	CoverURLs []string `json:"cover_urls"`
	// VideoPath is the work's MP4 after the change. The backend clears it
	// when the cover changes, because the rendered video embeds the old one.
	VideoPath string `json:"video_path"`
}

// ErrCoverOutcomeUnknown means the stream ended without saying whether the
// cover was made. The backend finishes — and bills — a generation even when
// the client has gone, so this is not "nothing happened" and must not be
// answered by asking again.
var ErrCoverOutcomeUnknown = errors.New("the cover stream ended without a result; the cover may still have been generated and billed")

// GenerateCover generates an AI cover and applies it, through the streaming
// endpoint. The synchronous one runs 30–60 seconds, past this client's
// request timeout; timing out there would abandon a generation the backend
// completes and bills anyway, and invite a second one. The stream carries
// keepalives and has no overall deadline.
func (c *Client) GenerateCover(ctx context.Context, p GenerateCoverParams) (*CoverResult, error) {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/works/generateCoverStream", p)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !isEventStream(resp) {
		return nil, decodeStreamRefusal(resp, "generate cover")
	}

	reader := sse.NewReader(resp.Body)
	for {
		ev, err := reader.Next()
		if err == io.EOF {
			return nil, ErrCoverOutcomeUnknown
		}
		if err != nil {
			return nil, fmt.Errorf("%w (%v)", ErrCoverOutcomeUnknown, err)
		}
		raw := strings.TrimSpace(ev.Data)
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var payload ssePayload
		if json.Unmarshal([]byte(raw), &payload) != nil {
			continue
		}
		var d struct {
			Type    string          `json:"type"`
			MsgType string          `json:"msg_type"`
			Msg     string          `json:"msg"`
			Data    json.RawMessage `json:"data"`
		}
		if json.Unmarshal(payload.Data, &d) != nil {
			continue
		}
		// A failure is reported before anything is saved, and the backend
		// bills only after saving, so an error frame means nothing was spent.
		if strings.EqualFold(d.MsgType, "ERROR") {
			return nil, &errs.Object{SchemaVersion: "1", Code: "business_error", Message: strings.TrimSpace(d.Msg)}
		}
		if d.Type == "cover_gen_completed" {
			var r CoverResult
			if err := json.Unmarshal(d.Data, &r); err != nil {
				return nil, fmt.Errorf("decode cover result: %w", err)
			}
			return &r, nil
		}
	}
}
