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

## Run Contract

Applies from the moment a run starts. Every rule here exists because
breaking it costs the user money or loses a run that was still going.

**Never run `vibeknow create` twice for the same request.** A second
`create` is a second billed render, always — it never recovers or resumes
the first one. If you have lost the ids, run `vibeknow jobs list`, then
`vibeknow video list`. Only start over when the CLI has told you in so many
words that it is safe (`resend_safe: true`, see exit 6 below).

**One process owns the stream.** `create` and `video wait` already poll the
backend, print progress, and collect the result. Do not start a second
`video status` loop alongside a running one, and do not wrap the command in
your own poller.

**Do not resend because it went quiet.** A render takes minutes and the
pipeline is legitimately silent through some of them. Slowness, an empty
patch of output, and a tool call that timed out on your side are all
compatible with a run that is about to succeed.

**Never merge the streams.** `2>&1` destroys the contract: stdout is the
result and stderr is everything else. Redirect them to separate files if
you need to keep them.

**Do not use `tail -n 20` as a cursor.** Events arrive in bursts. Read
stderr from a saved offset, or you will silently skip whole stages.

**A finished process is not a finished run.** The shell returning, a
background job ending, or a notification arriving tells you nothing about
the task. Read the exit code and stdout.

**Never invent an `--confirm` value or add `--yes` on your own initiative.**
When the CLI hands back a spend decision it has stopped precisely because
the choice is not yours to make. Relay it and wait.

## Core Concepts

- **Hero command**: `vibeknow create --from <source>` resolves input → uploads if needed → submits to figlens pipeline → streams progress → returns video URL.
- **--from accepts 3 input types**: `doc_id` (used directly), URL (auto-uploaded to vectoria), local file path (auto-uploaded).
- **Sync vs async**: Default is sync (blocks until done). `--async` returns task_id + session_id immediately.
- **NDJSON event stream**: `--output ndjson` emits structured progress events (schema_version: "1"). See [events.md](references/events.md).
- **6 pipeline stages**: `parse` → `outline` → `storyboard` → `tts` → `render` → `publish`.
- **session_id**: every `video` subcommand is addressed by a `(task_id, session_id)` pair, both returned by `create`. You do not have to carry them: `create` records the pair locally, so `vibeknow video wait` with no arguments reattaches to the most recent run and `vibeknow video wait <task_id>` looks up the session for you. Passing `--session-id` explicitly still works and always wins.
- **Run ledger**: `vibeknow jobs list` shows what this machine started. Use it instead of re-running `create` when you have lost a task_id — a second `create` is a second billed render. The ledger is per-machine; when it has nothing, the video commands fall back to the account's most recent run and say so on stderr.
- **stdout is the answer, stderr is the run**: with `--output json`, stdout carries exactly one JSON document and stderr carries live progress as `vk_event={...}` lines. You get both from one invocation — you do not have to choose between watching and parsing. Set `VIBEKNOW_EVENTS=1` to get the same lines in text mode, `VIBEKNOW_EVENTS=0` to suppress them.
- **Local artifacts**: `share_url` is a hosted page you cannot show anyone from a terminal. Pass `--preview-dir <dir>` to `create`, `video wait`, or `video export` and the cover still (and the MP4, once exported) land on disk, each announced by a `resource_ready` event carrying an absolute `local_path` to a fully written file. Hand each new `local_path` to the user once. Unchanged content is not re-announced, so re-running into the same directory is safe.
- **Spending requires consent**: `video export` renders an MP4 and costs credits. Without a terminal it does not decide for you — see **Spend Decisions** below.

## Quick Reference

| Command | Description |
|---------|-------------|
| `vibeknow create --from <source>` | Generate a video (sync by default) |
| `vibeknow video status <task_id> --session-id <sid>` | Get task status |
| `vibeknow video wait <task_id> --session-id <sid>` | Stream progress, block until done |
| `vibeknow video download <task_id> --dest out.mp4` | Download rendered video (`--dest` is the path; `--output` is the format) |
| `vibeknow jobs list [--active]` | Recorded runs, newest first |
| `vibeknow jobs get [task_id]` | One recorded run (default: most recent) |
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

### Find a run you lost track of

```bash
vibeknow jobs list --output json     # every recorded run, newest first
vibeknow jobs list --active          # only the ones still going
vibeknow video wait                  # reattach to the most recent
```

Reach for this before re-running `create`: re-creating a run that is
already going costs a second render.

### Download the result

```bash
vibeknow video download t_xxx --session-id s_yyy
# Default destination: <session_id>.mp4

vibeknow video download t_xxx --session-id s_yyy --dest ./my-video.mp4
vibeknow video download t_xxx --session-id s_yyy --dest ./my-video.mp4 --overwrite
```

## Exit Code Handling

| Exit | Meaning | Agent Action |
|------|---------|--------------|
| 0 | Success | Extract `video_url` from output |
| 1 | General error | Read stderr |
| 2 | Invalid arguments | Fix and retry. Covers unknown/misspelled flags, unknown subcommands, missing required flags, stray positional args, and bad enum values. stderr names the valid values, and suggests the closest flag when you typo one. Never re-send the same command unchanged. |
| 3 | Auth error (missing/expired/replaced credential) — fires on **every** command, not just `create` | Run `vibeknow auth status` to inspect credential source. Re-login with `vibeknow auth login` (interactive) or set `VIBEKNOW_TOKEN`. See **vibeknow-core** for profile/diagnostics if installed. |
| 4 | Retryable: rate limited, server error, or concurrency cap | Wait, then re-send the same command |
| 5 | Task failed, **not retryable** | Report error to user, do not retry |
| 6 | Stream interrupted, **task status unknown** | Read `error.detail.resend_safe` — see below. Default to reconnecting with `vibeknow video wait`, not re-submitting. |
| 7 | Partial success: preview is ready, the MP4 render failed | Report the preview `share_url`; retry only the export |
| 8 | **Blocked on a decision only the user can make** | Stop. Show the user the pending action, wait for an answer, then run its `resume_command` verbatim. |
| 130 | User interrupt (SIGINT) | — |

### Exit 6: is a resend safe?

Exit 6 means the CLI could not observe the outcome. That is not the same as
the run having failed, and re-running blind is how you pay twice. The error
envelope's `detail` answers the question directly:

| `delivery` | `resend_safe` | What it means | Do |
|------------|---------------|---------------|-----|
| `submitted` | `false` | The backend has this run; it is likely still going | Reattach with the `next_actions` command |
| `not_submitted` | `true` | The backend has no record; nothing was billed | Starting over is free |
| `indeterminate` | `false` | The CLI could not find out | Check with `vibeknow video list` before deciding |

Branch on `resend_safe`. When it is absent or false, do not re-run `create`.

## Minimum Evidence

Do not report a deliverable until you hold its evidence. These are not
interchangeable — each row needs everything in it, and a nearby fact is not
a substitute for the one you need.

| Deliverable | Minimum evidence |
|-------------|------------------|
| The video exists (previewable) | `create` or `video wait` exited **0**, and the payload has `preview.ready: true` with a `share_url` |
| An MP4 was rendered | Exit **0** from `video export`, and `export.status: "succeeded"` with a `video_path` |
| An MP4 is on this machine | The row above, **plus** `video download` exited 0 and the file at `--dest` is non-empty |
| A local still to show the user | A `resource_ready` event, **plus** the file at its `local_path` reads back |

Specifically, none of these follow from one another:

- `preview.ready` does **not** mean an MP4 exists — export is a separate,
  separately billed step.
- `export.status: "succeeded"` does **not** mean a file is on disk. It
  means a file exists on the backend.
- A `share_url` is **not** a video file. It is a web page.
- `export-status` exiting 0 does **not** mean the render succeeded. It is a
  single-shot reading; the outcome is in `export.status`, which may say
  `failed`. Only *blocking* commands (`create`, `video wait`, `video
  export`) put the outcome in their exit code.

## Spend Decisions

`video export` costs credits. With no terminal attached the CLI will not
decide for you: it exits **8**, having written the decision to stdout.

```json
{ "status": "blocked",
  "pending_actions": [{
    "action_id": "act_9f3c…", "type": "export_confirmation", "blocking": true,
    "message": "About to render MP4 …",
    "payload": { "session_id": "s_x", "credits": 1, "operation": "render_mp4" },
    "options": [{ "id": "confirm", "effect": "resume" },
                { "id": "cancel",  "effect": "none" }],
    "resume_command": "vk video export 42 --session-id s_x --confirm act_9f3c…" }] }
```

What to do:

1. Show the user `message` and `payload` — what it does and what it costs.
2. Wait for an actual answer. Do not pick a default.
3. On **confirm**, run `resume_command` exactly as given.
4. On **cancel** (`effect: "none"`), run nothing at all.

You cannot derive `action_id`; it is not guessable and it is bound to this
run and this price. If it is rejected (exit 2) the terms changed — re-run
without `--confirm`, show the user the new terms, and ask again.

`--yes` and `VIBEKNOW_ASSUME_YES=1` still bypass the gate. Use them only
when the user has already authorised this spend in advance. Reaching for
either to get past a block you just received is the thing this exists to
prevent.

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

The same events appear on **stderr** behind a `vk_event=` prefix whenever
`--output json` is in use, so you do not have to give up a parseable result
to watch a run. Two more types appear there when `--preview-dir` is set:

| Event | Extra Fields | Meaning |
|-------|-------------|---------|
| `resource_ready` | `asset_kind`, `local_path`, `bytes` | A complete local file. Give it to the user once. |
| `resource_preview_warning` | `asset_kind`, `code`, `message` | An artifact did not arrive. **Not** a failed run. |

`asset_kind` is `cover_image` or `video_playback`. `local_path` is absolute
and the file is fully written before the event fires. The remote URL is
deliberately never included — it is signed, and relaying one publishes a
credential.

See [events.md](references/events.md) for the complete field reference, engine differences, and parsing examples.

## References

- [commands.md](references/commands.md) — Full flag reference for all commands
- [events.md](references/events.md) — NDJSON task event schema
- [errors.md](references/errors.md) — Exit codes, error codes, Error Object schema
- [recipes.md](references/recipes.md) — Advanced: retry, recovery, batch, NDJSON parsing
