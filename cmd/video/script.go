package video

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
)

var flagScriptSessionID string

var scriptCmd = &cobra.Command{
	Use:   "script [task_id]",
	Short: "read a finished work's narration, shot by shot",
	Args:  cobra.MaximumNArgs(1),
	Long: `Print what a video actually says, along with the shot breakdown it is
divided into.

Everything a caller could previously learn about a finished work was about
the container — its title, duration, cover, share link. The narration lived
only inside the generation stream, which reports a character *count* and not
the words, so "what does this video say" had no answer once the run ended
except to watch it.

Read-only and free: this reads stored rows, re-runs nothing, and bills
nothing, so it is safe to call as often as a conversation needs.

JSON output carries the full breakdown per shot — narration, duration,
layout, and public URLs for the still, the narration audio and the
subtitles.`,
	Example: `  vk video script
  vk video script 42
  vk video script 42 --session-id s_abc --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, sessionID, err := resolveTarget(cmd.Context(), args, flagScriptSessionID)
		if err != nil {
			return err
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		// Shots are addressed by work_id while everything else in this
		// package is addressed by session; the work lookup bridges the two.
		work, err := c.GetWorkBySession(cmd.Context(), sessionID)
		if err != nil {
			return err
		}
		scenes, err := c.GetWorkScenes(cmd.Context(), work.ID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" || format == "ndjson" {
			items := make([]map[string]any, 0, len(scenes))
			var total float64
			for _, s := range scenes {
				total += s.DurationSec
				item := map[string]any{
					"scene_index":  s.SceneIndex,
					"name":         s.Name,
					"script_text":  s.ScriptText,
					"duration_sec": s.DurationSec,
					"layout_type":  s.LayoutType,
					"status":       s.Status,
				}
				// Only present what actually exists: a shot still rendering
				// has no audio yet, and an empty string reads like a broken
				// link rather than "not ready".
				if s.BGImageURL != "" {
					item["bg_image_url"] = s.BGImageURL
				}
				if s.TTSUrl != "" {
					item["tts_url"] = s.TTSUrl
				}
				if s.SRTUrl != "" {
					item["srt_url"] = s.SRTUrl
				}
				if len(s.ImageIndexes) > 0 {
					item["image_indexes"] = s.ImageIndexes
				}
				items = append(items, item)
			}
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"task_id":      taskID,
				"session_id":   sessionID,
				"work_id":      work.ID,
				"title":        work.Title,
				"scene_count":  len(scenes),
				"duration_sec": total,
				"scenes":       items,
				// The full narration as one block, because "read me the
				// script" is the common request and stitching it from the
				// array is work every caller would otherwise repeat.
				"script": joinScript(scenes),
			})
		}

		if len(scenes) == 0 {
			return clerr.New("this work has no shots recorded yet").
				WithCode(6).
				WithHint("a run that is still generating has nothing to read; check `vk video status` first")
		}

		out := cmd.OutOrStdout()
		if work.Title != "" {
			fmt.Fprintf(out, "%s\n\n", work.Title)
		}
		for _, s := range scenes {
			header := fmt.Sprintf("[%d]", s.SceneIndex)
			if s.Name != "" {
				header += " " + s.Name
			}
			if s.DurationSec > 0 {
				header += fmt.Sprintf("  (%.1fs)", s.DurationSec)
			}
			fmt.Fprintln(out, header)
			if text := strings.TrimSpace(s.ScriptText); text != "" {
				fmt.Fprintf(out, "%s\n", text)
			}
			fmt.Fprintln(out)
		}
		return nil
	},
}

// joinScript stitches the per-shot narration into one readable block, in shot
// order, skipping shots that carry no words (a title card, a transition).
func joinScript(scenes []figlens.WorkScene) string {
	parts := make([]string, 0, len(scenes))
	for _, s := range scenes {
		if text := strings.TrimSpace(s.ScriptText); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func init() {
	scriptCmd.Flags().StringVar(&flagScriptSessionID, "session-id", "", "session ID (default: looked up in the local run ledger)")
}
