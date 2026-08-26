package figlens

import (
	"context"
	"fmt"
)

// WorkScene is one shot of a finished work: what is said over it, how long it
// runs, and where its rendered pieces live.
//
// The narration is the part that matters most here. Until this was readable,
// the script existed only inside the generation stream — the progress events
// carry `script_chars`, a character *count* — so once a run finished there was
// no way to answer "what does this video actually say" short of watching it.
type WorkScene struct {
	ID         int64  `json:"id"`
	SceneIndex int    `json:"scene_index"`
	Name       string `json:"name"`
	// ScriptText is the narration for this shot, verbatim.
	ScriptText  string  `json:"script_text"`
	LayoutType  string  `json:"layout_type"`
	DurationSec float64 `json:"duration_sec"`
	Status      int     `json:"status"`
	Version     int     `json:"version"`
	// BGImageURL / TTSUrl / SRTUrl are public URLs for the rendered pieces:
	// the still behind the shot, its narration audio, its subtitles.
	BGImageURL string `json:"bg_image_url"`
	TTSUrl     string `json:"tts_url"`
	SRTUrl     string `json:"srt_url"`
	// ImageIndexes names the source images this shot draws on, and is only
	// populated on the illustrated (image) line.
	ImageIndexes []int `json:"image_indexes,omitempty"`
}

type workScenesResponse struct {
	WorkID int64       `json:"work_id"`
	Scenes []WorkScene `json:"scenes"`
}

// GetWorkScenes reads the shot list of a work's current version.
//
// Read-only and free: it queries stored rows rather than re-running anything,
// so it costs nothing and can be called as often as a conversation needs.
func (c *Client) GetWorkScenes(ctx context.Context, workID int64) ([]WorkScene, error) {
	var resp workScenesResponse
	path := fmt.Sprintf("/v1/works/scenes?work_id=%d", workID)
	if err := c.http.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get work scenes: %w", err)
	}
	return resp.Scenes, nil
}
