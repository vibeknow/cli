package figlens

// NDJSONFields renders the event into the canonical wire-shape map that
// `vk create` and `vk video wait` both emit on `--output ndjson`. The two
// commands deliberately use the same projection so a downstream agent
// can consume either stream interchangeably.
//
// Optional fields are omitted (not zero-emitted) so consumers can rely
// on presence implying a real value — useful in particular for
// `duration_ms`, which the agent engine simply does not produce.
func (e StreamEvent) NDJSONFields() map[string]any {
	switch e.Type {
	case "node.started", "node.succeeded", "node.failed", "node.warning":
		out := map[string]any{
			"type":    e.Type,
			"stage":   e.Stage,
			"node":    e.Node,
			"message": e.Message,
		}
		// Real node outputs (chapters, script_chars, duration_sec, …);
		// only success events carry them, and only some nodes produce them.
		if len(e.Metrics) > 0 {
			out["metrics"] = e.Metrics
		}
		return out
	case "node.progress":
		return map[string]any{
			"type":    e.Type,
			"status":  e.Status,
			"message": e.Message,
		}
	case "task.succeeded":
		out := map[string]any{
			"type":       e.Type,
			"session_id": e.SessionID,
		}
		if e.VideoURL != "" {
			out["video_url"] = e.VideoURL
		}
		if e.DurationMs > 0 {
			out["duration_ms"] = e.DurationMs
		}
		return out
	case "task.failed":
		return map[string]any{
			"type":      e.Type,
			"code":      e.Code,
			"message":   e.Message,
			"retryable": e.Retryable,
		}
	case "task.paused":
		return map[string]any{
			"type":    e.Type,
			"message": e.Message,
		}
	}
	// Unknown event types are passed through with just the type so
	// future backend additions don't crash existing consumers.
	return map[string]any{"type": e.Type}
}
