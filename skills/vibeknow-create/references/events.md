# NDJSON Task Event Schema

Schema version: `"1"` (see main spec §11.1 for canonical definition).

Produced by: `vibeknow create` (sync mode, non-TTY or `--output ndjson`) and `vibeknow video wait`.

## Common Fields

Every event line contains:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | string | yes | Always `"1"` |
| `ts` | string (RFC3339) | yes | Server-side event timestamp |
| `event` | string (enum) | yes | Event type (see below) |
| `task_id` | string | yes | Task identifier |

## Event Types

### task.submitted

Task accepted by the backend. No extra fields.

```json
{"schema_version":"1","ts":"2026-04-15T10:00:00Z","event":"task.submitted","task_id":"t_abc123"}
```

### task.queued

Task is waiting in queue.

| Field | Type | Description |
|-------|------|-------------|
| `queue_position` | int (optional) | Position in queue |

### stage.started

A pipeline stage has begun.

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | One of: `parse`, `outline`, `storyboard`, `tts`, `render`, `publish` |

### stage.progress

Progress update within a stage. Emitted at least once every 5 seconds.

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | Current stage |
| `percent` | int (0-100) | Completion percentage |
| `message` | string (optional) | Human-readable status |

### stage.succeeded

A pipeline stage completed successfully.

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | Completed stage |
| `duration_ms` | int | Stage duration in milliseconds |

### stage.failed

A pipeline stage failed.

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | Failed stage |
| `error_code` | string | Error code |
| `error_message` | string | Human-readable error |
| `retryable` | bool | Whether the failure is retryable |
| `fatal` | bool | If `true`, `task.failed` will follow. If `false`, pipeline may continue. |

### task.succeeded (**terminal**)

Video generation completed.

| Field | Type | Description |
|-------|------|-------------|
| `video_url` | string | URL to download the video |
| `thumbnail_url` | string (optional) | Thumbnail image URL |
| `duration_ms` | int | Total task duration in milliseconds |

```json
{"schema_version":"1","ts":"2026-04-15T10:05:00Z","event":"task.succeeded","task_id":"t_abc123","video_url":"https://cdn.example.com/v/t_abc123.mp4","duration_ms":300000}
```

### task.failed (**terminal**)

Task failed permanently.

| Field | Type | Description |
|-------|------|-------------|
| `failed_stage` | string | Stage that caused the failure |
| `error_code` | string | Error code |
| `error_message` | string | Human-readable error |
| `retryable` | bool | If `true` → exit code 4 (retry). If `false` → exit code 5 (give up). |

### task.cancelled (**terminal**)

Task was cancelled.

| Field | Type | Description |
|-------|------|-------------|
| `cancelled_by` | string | Who cancelled |

## Terminal Events

After receiving `task.succeeded`, `task.failed`, or `task.cancelled`, the CLI closes the stream and exits. No more events will follow.

## Parsing Example (bash + jq)

```bash
vibeknow create --from doc.pdf --output ndjson | while IFS= read -r line; do
  event=$(echo "$line" | jq -r '.event')
  case "$event" in
    task.succeeded)
      url=$(echo "$line" | jq -r '.video_url')
      echo "Video ready: $url"
      ;;
    task.failed)
      msg=$(echo "$line" | jq -r '.error_message')
      echo "Failed: $msg" >&2
      ;;
    stage.progress)
      pct=$(echo "$line" | jq -r '.percent')
      stage=$(echo "$line" | jq -r '.stage')
      echo "[$stage] $pct%" >&2
      ;;
  esac
done
```
