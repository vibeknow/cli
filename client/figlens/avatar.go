package figlens

import (
	"context"
	"fmt"
)

// Avatar reference prefixes. The backend keys strictly on these: sys_ is a
// public preset (operations-curated), ua_ is the caller's own trained
// avatar asset (must be activated).
const (
	AvatarRefSystemPrefix = "sys_"
	AvatarRefUserPrefix   = "ua_"
)

// Avatar circle-window bounds, mirrored from the backend (heightPx is the
// window diameter at a 1080p base). Out-of-range values are a hard 400 at
// the stream entry, so the CLI rejects them before spending an init call.
const (
	AvatarMinHeightPx = 120
	AvatarMaxHeightPx = 480
)

// AvatarCatalogItem is one public avatar from GET /v1/avatar/catalog,
// merged with the caller's saved display preference (position/size).
type AvatarCatalogItem struct {
	ID         string   `json:"id"` // "sys_<assetId>", pass to create --avatar
	Name       string   `json:"name"`
	ImageURL   string   `json:"imageUrl"`
	DemoURL    string   `json:"demoUrl,omitempty"`
	Style      string   `json:"style,omitempty"`  // 3d | 2d
	Gender     string   `json:"gender,omitempty"` // male | female — pick a matching voice
	VoiceID    string   `json:"voiceId,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MemberOnly bool     `json:"memberOnly,omitempty"`
	Position   string   `json:"position"`
	HeightPx   float64  `json:"heightPx"`
}

// ListAvatarCatalog fetches the public avatar presets.
func (c *Client) ListAvatarCatalog(ctx context.Context) ([]AvatarCatalogItem, error) {
	var items []AvatarCatalogItem
	if err := c.http.Do(ctx, "GET", "/v1/avatar/catalog", nil, &items); err != nil {
		return nil, fmt.Errorf("list avatar catalog: %w", err)
	}
	return items, nil
}

// UserAvatarAsset is one of the caller's own avatar assets from
// GET /v1/assets?type=avatar. Only StatusActive assets are usable with
// create --avatar; the other states exist so a user can see why theirs
// is not usable yet (training, pending paid activation, frozen).
type UserAvatarAsset struct {
	ID     int64  `json:"id"` // wire id; "ua_<id>" is the create --avatar ref
	Name   string `json:"name"`
	Status int    `json:"status"`
}

// User asset status values (shared user_asset table).
const (
	UserAssetStatusActive            = 1
	UserAssetStatusTraining          = 3
	UserAssetStatusTrainFailed       = 4
	UserAssetStatusPendingActivation = 5
	UserAssetStatusFrozen            = 6
)

// AvatarStatusLabel renders a user-asset status as a stable label.
func AvatarStatusLabel(status int) string {
	switch status {
	case UserAssetStatusActive:
		return "active"
	case UserAssetStatusTraining:
		return "training"
	case UserAssetStatusTrainFailed:
		return "train_failed"
	case UserAssetStatusPendingActivation:
		return "pending_activation"
	case UserAssetStatusFrozen:
		return "frozen"
	}
	return fmt.Sprintf("status_%d", status)
}

// ListMyAvatars fetches the caller's own trained avatar assets, all states.
func (c *Client) ListMyAvatars(ctx context.Context) ([]UserAvatarAsset, error) {
	var assets []UserAvatarAsset
	if err := c.http.Do(ctx, "GET", "/v1/assets?type=avatar", nil, &assets); err != nil {
		return nil, fmt.Errorf("list my avatars: %w", err)
	}
	return assets, nil
}

type retryAvatarScenesRequest struct {
	SessionID string `json:"session_id"`
	// SceneIndex retries just that scene; nil retries every failed scene.
	SceneIndex *int `json:"scene_index,omitempty"`
}

type retryAvatarScenesResponse struct {
	RetryCount int `json:"retry_count"`
}

// RetryAvatarScenes re-runs the avatar render for a work's failed scenes
// (that alone — no pipeline re-run, no second charge). Returns how many
// scenes were reset; 0 means there was nothing failed to retry.
//
// This is the unblock path when `video export` is rejected with "N 幕数字
// 人生成失败": failed avatar scenes are terminal until retried, and the
// export gate refuses to render a video with blank avatar windows.
func (c *Client) RetryAvatarScenes(ctx context.Context, sessionID string, sceneIndex *int) (int, error) {
	var resp retryAvatarScenesResponse
	body := retryAvatarScenesRequest{SessionID: sessionID, SceneIndex: sceneIndex}
	if err := c.http.Do(ctx, "POST", "/v1/avatar/scenes/retry", body, &resp); err != nil {
		return 0, fmt.Errorf("retry avatar scenes: %w", err)
	}
	return resp.RetryCount, nil
}
