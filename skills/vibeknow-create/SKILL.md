---
name: vibeknow-create
version: 0.3.1
description: "Generate videos from documents/URLs/files, track video task progress, download results, list voice templates. Use when: user wants to create a video, check task status, download video, or browse voices."
metadata:
  requires:
    bins: ["vibeknow"]
  cliHelp: "vibeknow --help"
---

# vibeknow-create (v0.3.0)

## TRIGGER

- User wants to generate a video from a document, URL, or file
- Check video task status or wait for completion
- Download a rendered video
- List available voice templates

## SKIP

- Document upload/status only (no video) → use **vibeknow-doc**
- Auth, profile, config, diagnostics → use **vibeknow-core**

## Core Concepts

- **Hero command**: `vibeknow create --from <source>` resolves input → uploads if needed → submits to figlens pipeline → streams progress → returns video URL.
- **--from accepts 3 input types**: `doc_id` (used directly), URL (auto-uploaded to vectoria), local file path (auto-uploaded).
- **Sync vs async**: Default is sync (blocks until done). `--async` returns task_id + session_id immediately.
- **NDJSON event stream**: `--output ndjson` emits structured progress events (schema_version: "1"). See [events.md](references/events.md).
- **6 pipeline stages**: `parse` → `outline` → `storyboard` → `tts` → `render` → `publish`.
- **session_id**: All `video` subcommands require both `<task_id>` and `--session-id`. These are returned by `create`.

## Quick Reference

| Command | Description |
|---------|-------------|
| `vibeknow create --from <source>` | Generate a video (sync by default) |
| `vibeknow video status <task_id> --session-id <sid>` | Get task status |
| `vibeknow video wait <task_id> --session-id <sid>` | Stream progress, block until done |
| `vibeknow video download <task_id> --session-id <sid>` | Download rendered video |
| `vibeknow voice list` | List available voice templates |

For full flags and output examples, see [commands.md](references/commands.md).

## Common Tasks

### Generate a video (sync, simplest path)

```bash
vibeknow create --from slides.pdf
# Blocks until done, prints video URL
```

### Generate with specific voice

```bash
vibeknow voice list                              # find voice ID
vibeknow create --from slides.pdf --voice v_warm_female
```

### Async submit, then follow up

```bash
# Submit and exit immediately
vibeknow create --from https://example.com/doc --async
# Output: task_id=t_xxx session_id=s_yyy

# Later: check status
vibeknow video status t_xxx --session-id s_yyy

# Or: wait for completion
vibeknow video wait t_xxx --session-id s_yyy
```

### Agent mode (NDJSON streaming)

```bash
vibeknow create --from doc_abc --output ndjson
# Each line is a JSON event: task.submitted, stage.started, stage.progress, ...
# Terminal event: task.succeeded (with video_url) or task.failed
```

### Download the result

```bash
vibeknow video download t_xxx --session-id s_yyy
# Default output: <session_id>.mp4

vibeknow video download t_xxx --session-id s_yyy --output ./my-video.mp4
vibeknow video download t_xxx --session-id s_yyy --output ./my-video.mp4 --overwrite
```

## Exit Code Handling

| Exit | Meaning | Agent Action |
|------|---------|--------------|
| 0 | Success | Extract `video_url` from output |
| 1 | General error | Read stderr |
| 2 | Invalid arguments | Fix command syntax |
| 3 | Auth error | → vibeknow-core: check credentials |
| 4 | Task failed, **retryable** | Re-submit the same `create` command |
| 5 | Task failed, **not retryable** | Report error to user, do not retry |
| 6 | Stream interrupted, **task status unknown** | `vibeknow video wait <task_id> --session-id <sid>` to reconnect. Do NOT re-submit. |
| 130 | User interrupt (SIGINT) | — |

For detailed error handling and recovery, see [errors.md](references/errors.md) and [recipes.md](references/recipes.md).

## NDJSON Event Summary

Events share common fields: `schema_version`, `ts`, `event`, `task_id`.

Key events:

| Event | Extra Fields | Meaning |
|-------|-------------|---------|
| `task.submitted` | — | Task accepted |
| `stage.started` | `stage` | Pipeline stage begins |
| `stage.progress` | `stage`, `percent`, `message?` | Progress update |
| `stage.succeeded` | `stage`, `duration_ms` | Stage done |
| `task.succeeded` | `video_url`, `duration_ms` | **Terminal**: video ready |
| `task.failed` | `failed_stage`, `error_code`, `error_message`, `retryable` | **Terminal**: task failed |

See [events.md](references/events.md) for the complete list (including `task.queued`, `stage.failed`, `task.cancelled`) and parsing examples.

## References

- [commands.md](references/commands.md) — Full flag reference for all commands
- [events.md](references/events.md) — NDJSON task event schema
- [errors.md](references/errors.md) — Exit codes, error codes, Error Object schema
- [recipes.md](references/recipes.md) — Advanced: retry, recovery, batch, NDJSON parsing
