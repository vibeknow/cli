---
name: vibeknow-create
description: "Generate videos from documents/URLs/files, track video task progress, download results, list voice templates. Use when: user wants to create a video, check task status, download video, or browse voices."
version: 0.9.0
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
- **Four ways to name the source**: `--from` takes a `doc_id` (used directly), a URL (auto-uploaded to vectoria), a local file path (auto-uploaded), or `-` to read the text from stdin. `--text` takes the text itself, for a passage the user pasted rather than a file they pointed at. What it cannot do is invent material: a request with no source ("做个讲量子计算的视频") has no command — ask for it.
- **Creation modes** mirror the product's five online modes: default (灵活创作), `--engine agent` (一键成片), `--mode image` (图解视频), `--mode replica` (PPT 讲解), `--mode handdraw` (手绘动画). 原稿锁定 is the orthogonal `--script-lock` switch, combinable with any mode. Style via `--theme` (browse with `vibeknow theme list --mode <mode>`), output language via `--language`.
- **Sync vs async**: Default is sync (blocks until done). `--async` returns task_id + session_id immediately.
- **Waiting in bounded steps**: `vibeknow video wait --for 90s` watches the event stream for that long, then reports the stage it reached and exits 6 with `reason: "wait_budget_expired"`. Run it again to keep waiting. Use it when your own tool times out sooner than a render takes — `video status` cannot substitute, since the work row carries no progress until the export stage.
- **NDJSON event stream**: `--output ndjson` emits structured progress events (schema_version: "1"). See [events.md](references/events.md).
- **4 pipeline stages**: `outline` → `tts` → `render` → `publish`. Not every node reports progress: parts of every run — and the entire middle of a `handdraw` run — are silent by design. Silence is not a hang; wait for a terminal event.
- **session_id**: every `video` subcommand is addressed by a `(task_id, session_id)` pair, both returned by `create`. You do not have to carry them: `create` records the pair locally, so `vibeknow video wait` with no arguments reattaches to the most recent run and `vibeknow video wait <task_id>` looks up the session for you. Passing `--session-id` explicitly still works and always wins.
- **Run ledger**: `vibeknow jobs list` shows what this machine started. Use it instead of re-running `create` when you have lost a task_id — a second `create` is a second billed render. The ledger is per-machine; when it has nothing, the video commands fall back to the account's most recent run and say so on stderr.
- **stdout is the answer, stderr is the run**: with `--output json`, stdout carries exactly one JSON document and stderr carries live progress as `vk_event={...}` lines. You get both from one invocation — you do not have to choose between watching and parsing. Set `VIBEKNOW_EVENTS=1` to get the same lines in text mode, `VIBEKNOW_EVENTS=0` to suppress them.
- **Local artifacts**: `share_url` is a hosted page you cannot show anyone from a terminal. Pass `--preview-dir <dir>` to `create`, `video status`, `video wait`, or `video export` and the cover still (and the MP4, once exported) land on disk, each announced by a `resource_ready` event carrying an absolute `local_path` to a fully written file. On `status` this is the only route open to a caller that did not start the run — reattaching to someone else's task, or coming back after losing its own context. Hand each new `local_path` to the user once. Unchanged content is not re-announced, so re-running into the same directory is safe.
- **Spending requires consent**: `video export` renders an MP4 and costs credits. Without a terminal it does not decide for you — see **Spend Decisions** below.

## Quick Reference

| Command | Description |
|---------|-------------|
| `vibeknow create --from <source>` | Generate a video (sync by default) |
| `vibeknow create --text "<text>"` | Same, from text the user pasted (`--from -` for stdin) |
| `vibeknow create --from <source> --preset <name>` | Same, with a saved style bundle; command-line flags still win |
| `vibeknow video status <task_id> --session-id <sid>` | Get task status (`--preview-dir` to also fetch the artifacts) |
| `vibeknow video wait <task_id> --session-id <sid>` | Stream progress, block until done |
| `vibeknow video wait <task_id> --for 90s` | Same, but come back after 90s with the stage reached (exit 6) |
| `vibeknow video download <task_id> --dest out.mp4` | Download rendered video (`--dest` is the path; `--output` is the format) |
| `vibeknow jobs list [--active]` | Recorded runs, newest first |
| `vibeknow jobs get [task_id]` | One recorded run (default: most recent) |
| `vibeknow voice list [--language <locale>]` | Voices: public templates grouped by language + your cloned voices |
| `vibeknow theme list --mode <mode>` | Visual themes/styles usable with `create --theme` |
| `vibeknow avatar list` | Talking-head presenters (public presets + your trained ones) for `create --avatar` |
| `vibeknow subtitle fonts` | Font families `video set --subtitle-font` accepts (free) |
| `vibeknow subtitle presets` | Ready-made subtitle looks for `video set --subtitle-preset` (free) |
| `vibeknow video avatar-retry [task_id]` | Retry failed avatar scenes (unblocks a rejected export; no re-charge) |
| `vibeknow video script [task_id]` | Read what the video says, shot by shot (free) |
| `vibeknow video edit [task_id] --scene N --script "…"` | Rewrite one shot's narration (**bills**; confirmation gate) |

For full flags and output examples, see [commands.md](references/commands.md).

## Common Tasks

### Generate a video (sync, simplest path)

```bash
vibeknow create --from slides.pdf
# Blocks until done, prints video URL
```

### Pick a creation mode

```bash
vibeknow create --from slides.pdf --mode replica     # PPT 讲解: page-by-page replay (PDF/PPT only)
vibeknow create --from notes.md --mode image --pages 6   # 图解视频: one AI illustration per page
vibeknow create --from story.md --mode handdraw      # 手绘动画 (silent mid-run is normal)
vibeknow create --from script.md --script-lock       # 原稿锁定: narrate the document verbatim
vibeknow create --from doc.pdf --engine agent        # 一键成片 (v2 agent engine)
```

### Style, language, voice

```bash
vibeknow theme list --mode image                 # browse the mode's style catalog
vibeknow create --from notes.md --mode image --theme <theme_id>
vibeknow voice list --language en-US             # voices are grouped by language
vibeknow create --from slides.pdf --voice <speech_voice_id> --language en-US
```

### Reuse a saved style (`--preset`)

```bash
vibeknow create --from deck.pdf --preset brand-explainer      # <config>/presets/brand-explainer.yaml
vibeknow create --from deck.pdf --preset ./team/shorts.yaml   # or a path
vibeknow create --from deck.pdf --preset brand --mode replica # your --mode wins over the file's
```

A preset is a YAML file bundling style flags (`mode`, `aspect`, `theme`,
`voice`, `language`, `bgm`, `pages`, `images`, `prompt`, `avatar*`,
`script-lock`, `engine`). It supplies **defaults only** — every flag you also
pass on the command line wins.

A preset can never approve a spend: `export`, `yes` and `confirm` are refused
with exit 2, as are `from`/`kb-id` and `async`/`preview-dir`/`output`. Same
for any unknown key. All of it is checked before the first upload, so a bad
preset costs nothing — read the error, fix the file, re-run.

If the user asks what a run used, read the `preset.applied` event (or the
`preset "<name>" applied: …` stderr line): its `keys` list is exactly what
the file contributed, with command-line overrides already excluded. Do not
infer it from the file.

### Fix a line of narration (`video edit`)

```bash
vibeknow video script 42                                       # free: read the shots and their numbers
vibeknow video edit 42 --scene 3 --script "换一种说法，更短一点。"   # exit 8: shows the diff, asks
vibeknow video edit 42 --scene 3 --script "…" --confirm act_…   # after the user agrees
```

The only content edit there is. One shot per call — three shots is three
confirmations and three charges. `--scene` uses the numbers `video script`
prints, counting from 1.

Add `--script-only` to regenerate just the voice-over (cheaper, layout
untouched). Leave it off — the default — to rebuild the shot's layout and
background image too, which is what a rewrite of a noticeably different
length needs, since nothing re-flows without it.

The block payload carries `from` and `to`. **Show the user both**: they are
approving a diff, and there is no undo — no endpoint returns a previous
version of a shot. The token is bound to that exact text, so reworded
attempts each get their own block.

Afterwards the preview and share link are current but the rendered MP4 is
**not**: the backend leaves the old file in place, so `video download` still
returns the previous narration until the next `video export`. The response
flags this as `export_stale`.

Exit 4 means another edit holds the lock on this work — wait a moment and
retry the same command.

### Make the subtitles readable (`video set`)

```bash
vibeknow subtitle presets                                  # free: see the looks and what each one sets
vibeknow video set 42 --subtitle-preset 2                  # apply one
vibeknow video set 42 --subtitle-preset 2 --subtitle-size 52   # that look, but bigger
```

**Use a preset rather than assembling a look from the individual flags.**
Readability is a combination: the outlined looks also clear the background
plate, and the plated looks also switch the outline off. Set one half without
the other and the command still exits 0 — the video just looks wrong, and
nothing reports it.

A preset touches only the fields that make up its look; size, vertical
position and entry animation stay as the work had them. Any `--subtitle-*`
flag you pass alongside wins over the preset.

For a font, `vibeknow subtitle fonts` lists every family that will be
accepted; pass the `#`. A guess is refused server-side with a message that
does not say what the alternatives are, so do not guess.

Free and immediate, like the rest of `video set` — but it still discards the
rendered MP4 (`export_invalidated`), so a re-`export` is needed to get a file
that matches.

### Talking-head avatar

```bash
vibeknow avatar list                             # sys_<id> presets + your own ua_<id>
vibeknow create --from slides.pdf --avatar sys_7 --voice <its VOICE_ID>
vibeknow create --from slides.pdf --avatar ua_12 --avatar-position bottom-right --avatar-size 300
```

Not available with `--mode handdraw` or `--engine agent` (rejected with
exit 2 — the backend would silently render without the presenter). Each
public preset carries a paired `VOICE_ID`; using it keeps face and voice
matched. **Export gating**: `video export` is refused while any scene's
avatar is still rendering ("生成中") or has failed ("生成失败"). Failed
scenes stay failed until retried — run `vibeknow video avatar-retry`,
wait, then export again. The retry re-bills nothing.

### Use text the user pasted

```bash
vibeknow create --text "季度复盘要点…" --async --output json

# Long or multi-line: stdin, so the shell never touches the text
vibeknow create --from - --script-lock --async --output json <<'EOF'
第一段讲稿…
第二段讲稿…
EOF
```

`--script-lock` on a paste is "照着我这段话念". Without it the text is source
material and a script gets written from it.

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

### Follow a run when you cannot block for minutes

```bash
vibeknow create --from report.pdf --async --output json
# → {"task_id":42,"session_id":"s_yyy"}

vibeknow video wait 42 --session-id s_yyy --for 90s --output json
# exit 6 + reason "wait_budget_expired" → report the stage, run it again
# exit 0 → done; the snapshot is on stdout
```

Each call comes back inside the budget with a stage worth repeating, instead
of one long block your own timeout would cut. Repeating the call costs
nothing: the run belongs to the backend, not to the process watching it.
Never answer a spent budget with a second `create`.

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
| `node.succeeded` | `stage`, `node`, `message`, `metrics?` | Node done; `metrics` (when present) carries real outputs, e.g. `script_chars`, `duration_sec` |
| `node.failed` | `stage`, `node`, `message` | Node failed (not necessarily terminal — wait for `task.failed`) |
| `task.succeeded` | `session_id`, `video_url`, `duration_ms` | **Terminal**: video ready |
| `task.failed` | `code`, `message`, `retryable` | **Terminal**: task failed (`retryable=true` → exit 4, `false` → exit 5) |
| `task.paused` | `message` | Run paused (web editor's pause, or `vibeknow video pause`). **Not** a failure: continue it with `vibeknow video resume <task_id>` — never by creating the video again, which bills in full. The command exits 6. |

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
