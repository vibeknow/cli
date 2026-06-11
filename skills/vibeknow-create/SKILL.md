---
name: vibeknow-create
description: "Generate videos from documents/URLs/files, track video task progress, download results, list voice templates. Use when: user wants to create a video, check task status, download video, or browse voices."
version: 0.7.1
emoji: "🎬"
homepage: https://github.com/vibeknow/cli
allowed-tools: Bash(vibeknow:*)
metadata:
  openclaw:
    requires:
      bins: ["vibeknow"]
    install:
      - kind: node
        package: vibeknow-cli
        bins: [vibeknow]
    primaryEnv: VIBEKNOW_TOKEN
    envVars:
      - name: VIBEKNOW_TOKEN
        required: false
        description: "API token. Optional — if unset, the CLI uses credentials configured via `vibeknow auth login` (managed by vibeknow-core)."
      - name: VIBEKNOW_EXPORT_TIMEOUT
        required: false
        description: "Override the default 15-minute timeout for synchronous video export polling (Go duration format, e.g. `30m`)."
---

# vibeknow-create

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
| 3 | Auth error | Run `vibeknow auth status` to inspect credential source. Re-login with `vibeknow auth login` (interactive) or set `VIBEKNOW_TOKEN`. See **vibeknow-core** for profile/diagnostics if installed. |
| 4 | Task failed, **retryable** | Re-submit the same `create` command |
| 5 | Task failed, **not retryable** | Report error to user, do not retry |
| 6 | Stream interrupted, **task status unknown** | `vibeknow video wait <task_id> --session-id <sid>` to reconnect. Do NOT re-submit. |
| 130 | User interrupt (SIGINT) | — |

For detailed error handling and recovery, see [errors.md](references/errors.md) and [recipes.md](references/recipes.md).

## NDJSON Event Summary

Events share common fields: `schema_version`, `ts`, `type`.

Key events (pipeline engine):

| Event | Extra Fields | Meaning |
|-------|-------------|---------|
| `node.started` | `stage`, `node`, `message` | Pipeline node begins |
| `node.succeeded` | `stage`, `node`, `message` | Node done |
| `node.failed` | `stage`, `node`, `message` | Node failed (not necessarily terminal — wait for `task.failed`) |
| `task.succeeded` | `session_id`, `video_url`, `duration_ms` | **Terminal**: video ready |
| `task.failed` | `code`, `message`, `retryable` | **Terminal**: task failed (`retryable=true` → exit 4, `false` → exit 5) |

Agent engine (`--engine agent`) replaces `node.started/succeeded/failed` with `node.progress` carrying `status` + `message`, and omits `duration_ms` from `task.succeeded`.

See [events.md](references/events.md) for the complete field reference, engine differences, and parsing examples.

## References

- [commands.md](references/commands.md) — Full flag reference for all commands
- [events.md](references/events.md) — NDJSON task event schema
- [errors.md](references/errors.md) — Exit codes, error codes, Error Object schema
- [recipes.md](references/recipes.md) — Advanced: retry, recovery, batch, NDJSON parsing
