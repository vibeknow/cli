package figlens

import (
	"context"
	"encoding/json"
	"fmt"
)

// Settings that can be changed on a finished work without regenerating it:
// background music, subtitles, and the task's title.
//
// Every one of these clears the work's rendered MP4 server-side — the change
// has to be baked in, and the existing file no longer reflects the work. The
// preview and share link stay live; only the downloadable file goes, and it
// comes back on the next (billed) export. Callers have to say so, or a user
// who turned the music off finds their download quietly gone.

const (
	BgmOn       = "on"
	BgmOff      = "off"
	SubtitleOn  = "on"
	SubtitleOff = "off"

	// BGMVolumeMin / BGMVolumeMax mirror the backend's accepted range.
	// Checking here turns a round trip that ends in a 400 into an argument
	// error the caller can fix without one.
	BGMVolumeMin = 0.1
	BGMVolumeMax = 2.0
)

// SubtitleStyle is the whole-video subtitle appearance.
//
// The backend stores exactly what it is sent — it does not merge with what is
// already there — so this must always be populated from the work's current
// style (ParseSubtitleStyle) before a field is changed.
type SubtitleStyle struct {
	FontFamily      string  `json:"fontFamily,omitempty"`
	FontSize        int     `json:"fontSize,omitempty"`
	FontWeight      int     `json:"fontWeight,omitempty"`
	Color           string  `json:"color,omitempty"`
	BackgroundColor string  `json:"backgroundColor,omitempty"`
	BottomPercent   float64 `json:"bottomPercent,omitempty"`
	StrokeColor     string  `json:"strokeColor,omitempty"`
	StrokeWidth     float64 `json:"strokeWidth,omitempty"`
	Animation       string  `json:"animation,omitempty"`
}

// ParseSubtitleStyle reads a work's stored style. An absent or unreadable
// value yields the zero style rather than an error: a work that has never had
// its subtitles styled simply has none, which is a starting point, not a
// failure.
func ParseSubtitleStyle(raw string) SubtitleStyle {
	var s SubtitleStyle
	if raw == "" {
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s)
	return s
}

type bgmSwitchRequest struct {
	ID  int64  `json:"id"`
	Bgm string `json:"bgm"`
}

type bgmVolumeRequest struct {
	ID     int64   `json:"id"`
	Volume float64 `json:"volume"`
}

type subtitleSwitchRequest struct {
	ID       int64  `json:"id"`
	Subtitle string `json:"subtitle"`
}

type subtitleStyleRequest struct {
	ID            int64         `json:"id"`
	SubtitleStyle SubtitleStyle `json:"subtitleStyle"`
}

type renameTaskRequest struct {
	TaskID int64  `json:"task_id"`
	Title  string `json:"title"`
}

// SetBGM turns the work's background music on or off.
func (c *Client) SetBGM(ctx context.Context, workID int64, state string) error {
	if state != BgmOn && state != BgmOff {
		return fmt.Errorf("bgm must be %q or %q, got %q", BgmOn, BgmOff, state)
	}
	if err := c.http.Do(ctx, "POST", "/v1/works/bgmSwitch",
		bgmSwitchRequest{ID: workID, Bgm: state}, nil); err != nil {
		return fmt.Errorf("set bgm: %w", err)
	}
	return nil
}

// SetBGMVolume scales the background music. Not every engine supports it; the
// backend refuses the ones that do not rather than clearing the rendered file
// for a change it would never bake in.
func (c *Client) SetBGMVolume(ctx context.Context, workID int64, volume float64) error {
	if volume < BGMVolumeMin || volume > BGMVolumeMax {
		return fmt.Errorf("bgm volume must be between %g and %g, got %g", BGMVolumeMin, BGMVolumeMax, volume)
	}
	if err := c.http.Do(ctx, "POST", "/v1/works/bgmVolume",
		bgmVolumeRequest{ID: workID, Volume: volume}, nil); err != nil {
		return fmt.Errorf("set bgm volume: %w", err)
	}
	return nil
}

// SetSubtitle turns subtitles on or off for the whole video.
func (c *Client) SetSubtitle(ctx context.Context, workID int64, state string) error {
	if state != SubtitleOn && state != SubtitleOff {
		return fmt.Errorf("subtitle must be %q or %q, got %q", SubtitleOn, SubtitleOff, state)
	}
	if err := c.http.Do(ctx, "POST", "/v1/works/subtitleSwitch",
		subtitleSwitchRequest{ID: workID, Subtitle: state}, nil); err != nil {
		return fmt.Errorf("set subtitle: %w", err)
	}
	return nil
}

// SetSubtitleStyle replaces the whole-video subtitle style.
//
// style must be complete. The backend marshals what it receives straight into
// storage, so anything left out is cleared: build it from ParseSubtitleStyle
// on the work's current style, then change only what the caller asked for.
func (c *Client) SetSubtitleStyle(ctx context.Context, workID int64, style SubtitleStyle) error {
	if err := c.http.Do(ctx, "POST", "/v1/works/subtitleStyle",
		subtitleStyleRequest{ID: workID, SubtitleStyle: style}, nil); err != nil {
		return fmt.Errorf("set subtitle style: %w", err)
	}
	return nil
}

// RenameTask changes a task's title. Unlike the rest of this file it touches
// no rendered output, so it does not invalidate anything.
func (c *Client) RenameTask(ctx context.Context, taskID int64, title string) error {
	if err := c.http.Do(ctx, "POST", "/v1/tasks/updateTitle",
		renameTaskRequest{TaskID: taskID, Title: title}, nil); err != nil {
		return fmt.Errorf("rename task: %w", err)
	}
	return nil
}
