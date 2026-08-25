# NDJSON Task Event Schema

Schema version: `"1"`.

Produced by: `vibeknow create --output ndjson` and (for already-submitted tasks) `vibeknow video wait --output ndjson`.

## Two channels, one shape

The same events reach a consumer two ways:

| Channel | When | Where |
|---------|------|-------|
| NDJSON stream | `--output ndjson` | **stdout**, one bare JSON object per line |
| Structured progress | `--output json` (or `VIBEKNOW_EVENTS=1`) | **stderr**, each line prefixed `vk_event=` |

The payload after the prefix is byte-identical in shape to an NDJSON line —
same `schema_version`, `ts`, `type` and event-specific fields — so one
parser serves both. The prefix exists because stderr also carries human
text; a consumer picks the machine lines out by string match rather than
attempting to JSON-parse every line it sees.

The point of the stderr channel is that `--output json` no longer costs you
visibility: stdout carries exactly one document (the final snapshot) while
stderr carries the run as it happens. With `--output ndjson` the stream
stays on stdout as before and nothing is duplicated onto stderr.

Two event types appear **only** on the stderr channel, and only when
`--preview-dir` is set:

### resource_ready

A local artifact is written and complete.

| Field | Type | Description |
|-------|------|-------------|
| `asset_kind` | string | `cover_image` or `video_playback` |
| `session_id` | string | The run this belongs to |
| `local_path` | string | **Absolute** path to a fully written file |
| `bytes` | number | File size |

Give each new `local_path` to the user exactly once. The event never
carries the remote URL: those are signed, and relaying one publishes a
credential.

Content is deduplicated against what is already at the destination, so
re-running a command into the same directory does not re-announce
unchanged bytes. A changed artifact does emit a new event.

### resource_preview_warning

An artifact did not arrive. **The run did not fail.**

| Field | Type | Description |
|-------|------|-------------|
| `asset_kind` | string | Which artifact |
| `session_id` | string | The run |
| `code` | string | `download_failed` or `resolve_failed` |
| `message` | string | Detail |

Do not report this as a failed render. If the user specifically needed that
artifact, say it is unavailable and continue.

This document describes the events the CLI **actually emits today**. The earlier draft of this file documented several events (`task.submitted`, `task.queued`, `stage.*`, `task.cancelled`) that the backend has never produced — they were aspirational and have been removed. A future schema version may reintroduce them once the backend starts emitting them; consumers should ignore unknown event types.

## Common Fields

Every event line contains:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | string | yes | Always `"1"` |
| `ts` | string (RFC3339Nano) | yes | Client-side event timestamp (CLI clock) |
| `type` | string (enum) | yes | Event type (see below). Wire field is `type`, not `event`. |

The CLI also stamps event-specific fields documented below.

## Engine differences

The backend offers two engines selectable via `--engine`:

- **`pipeline`** (default, v=3): structured stage graph. Emits `node.started` / `node.succeeded` / `node.failed` for each named pipeline node. Terminal `task.succeeded` carries `video_url` *and* `duration_ms`.
- **`agent`** (v=2): free-form agent loop, no stage graph. Emits `node.progress` lines instead of `node.*` lifecycle events. Terminal `task.succeeded` carries `video_url` but **not** `duration_ms` (the backend does not include it in the agent-mode aim_result payload).

Consumers that need cross-engine compatibility should treat `duration_ms` as optional.

## Event Types

### node.started

A pipeline node has begun (pipeline engine only).

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | High-level stage bucket (e.g. `parse`, `outline`, `storyboard`, `tts`, `render`, `publish`) |
| `node` | string | Specific node display name within the stage |
| `message` | string | Backend-supplied status message (may be empty) |

```json
{"schema_version":"1","ts":"2026-05-15T10:00:01.234Z","type":"node.started","stage":"parse","node":"prepare","message":"开始解析"}
```

### node.succeeded

A pipeline node completed (pipeline engine only). Same fields as `node.started`.

### node.failed

A pipeline node failed (pipeline engine only). Same fields as `node.started`. A non-fatal node failure does **not** by itself end the stream — wait for the terminal `task.failed`. There is currently no `fatal` or `retryable` field on this event.

### node.progress

Free-form progress message (agent engine only). The agent does not have a named stage graph, so progress is reported as a status string + human-readable message.

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `"start"` / `"success"` / `"error"` |
| `message` | string | Human-readable description of the current step |

```json
{"schema_version":"1","ts":"2026-05-15T10:00:01.234Z","type":"node.progress","status":"start","message":"正在调用知识库..."}
```

### task.succeeded (**terminal**)

Video generation completed. Always the last event emitted on success.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Session that produced the video (use with `vk video download`) |
| `video_url` | string (optional) | Playable HTML URL. Omitted if the backend did not include one. |
| `duration_ms` | integer (optional) | Rendered video length in milliseconds. **Pipeline engine only** — omitted on agent engine. |

```json
{"schema_version":"1","ts":"2026-05-15T10:05:00.000Z","type":"task.succeeded","session_id":"s_abc","video_url":"https://cdn.example.com/v/s_abc.html","duration_ms":42500}
```

### task.failed (**terminal**)

Task failed permanently. Always the last event emitted on failure — either inside the SSE stream (`stage.failed`-like conditions, backend `error` event, business-code SSE envelope) or **synthesized by the CLI for pre-stream failures** (e.g. when `/v1/tasks/init` rejects with a business code before the SSE stream opens). Consumers should treat both sources identically; the wire shape is the same.

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Stable error code (e.g. `insufficient_credits`, `script_invalid`, `concurrent_work_limit`, `rate_limited`, `internal_error`, `business_error`). Empty when the backend emitted a plain `error` SSE event with no envelope code. |
| `message` | string | Human-readable failure reason from the backend |
| `retryable` | boolean | `true` if re-running the same `create` command is likely to succeed (transient codes: `rate_limited`, `internal_error`, `concurrent_work_limit`). `false` for permanent codes (e.g. `script_invalid`, `insufficient_credits`) and for any failure where the CLI cannot prove a retry would help. Determines exit code: `true` → exit 4, `false` → exit 5. |

The `retryable` flag is **derived on the CLI side** from `code` — the backend does not currently include one on its terminal error event. The mapping is intentionally conservative: when in doubt, `retryable` is `false`.

```json
{"schema_version":"1","ts":"2026-05-15T10:05:00.000Z","type":"task.failed","code":"insufficient_credits","message":"积分不足","retryable":false}
```

## Terminal Events

After receiving `task.succeeded` or `task.failed`, the CLI closes the stream and exits. No more events will follow.

`task.cancelled` is not currently emitted — SIGINT terminates the process with exit 130 without producing a terminal NDJSON event.

## Parsing Example (bash + jq)

```bash
vibeknow create --from doc.pdf --output ndjson | while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  case "$type" in
    task.succeeded)
      url=$(echo "$line" | jq -r '.video_url // empty')
      [ -n "$url" ] && echo "Video ready: $url"
      ;;
    task.failed)
      msg=$(echo "$line" | jq -r '.message')
      retryable=$(echo "$line" | jq -r '.retryable')
      echo "Failed (retryable=$retryable): $msg" >&2
      ;;
    node.started|node.succeeded|node.failed)
      stage=$(echo "$line" | jq -r '.stage')
      node=$(echo "$line" | jq -r '.node')
      echo "[$stage] $type: $node" >&2
      ;;
    node.progress)
      msg=$(echo "$line" | jq -r '.message')
      echo "[agent] $msg" >&2
      ;;
  esac
done
```

## Field Stability

`type`, `schema_version`, `ts`, `session_id`, `code`, and `message` are stable: existing values won't be renamed within schema version `"1"`. New event types and new fields may be added without bumping the schema version — consumers must ignore unknown fields and unknown `type` values rather than treat them as errors.

A future schema version bump (to `"2"`) will be reserved for breaking changes such as renaming `code` → `error_code` or replacing `node.*` with `stage.*`.
