# vibeknow-create Command Reference

## Global Flags

| Flag | Description |
|------|-------------|
| `--output string` | Output format: `text\|json\|ndjson` (default `text`; `VIBEKNOW_OUTPUT` sets the default, an explicit flag wins). An unrecognized value exits 2 rather than falling back to text. |
| `--profile string` | Override active profile for this command |
| `--verbose` | Emit request/response summaries (credentials redacted) |

## Environment

| Variable | Effect |
|----------|--------|
| `VIBEKNOW_OUTPUT` | Default `--output` value; an explicit flag wins |
| `VIBEKNOW_EVENTS` | `1`/`on` forces `vk_event=` progress lines onto stderr in any format; `0`/`off` suppresses them. Unset means on for `--output json` only. |
| `VIBEKNOW_ASSUME_YES` | Pre-authorises paid actions, same as `--yes`. Only set it when the user has agreed to the spend in advance. |
| `VIBEKNOW_EXPORT_TIMEOUT` | Local deadline for synchronous export polling (default 15m). Bounds the **local wait only** — it does not cancel the backend render. |

---

## create

Resolve `--from` to a document, then generate a video via the figlens pipeline.

`--from` accepts:
- A `doc_id` (e.g. `doc_abc12345`) — used directly
- A URL (`http://` or `https://`) — uploaded to vectoria first
- A local file path — uploaded to vectoria first

```
vibeknow create [flags]
```

| Flag | Description |
|------|-------------|
| `--from string` | doc_id, URL, or local file path **(required)** |
| `--preset string` | Saved bundle of style options: a name under `<config>/presets/` or a path to a `.yaml` file. Supplies defaults only — anything also given on the command line wins. See **Presets** below. |
| `--mode string` | `replica` (PPT 讲解, page-by-page; PDF/PPT sources only), `image` (图解视频, AI illustration per page), `handdraw` (手绘动画). Omit for 灵活创作 (the default line). |
| `--script-lock` | 原稿锁定: narrate the document verbatim instead of writing a script. Orthogonal to `--mode` — combines with any of them. |
| `--engine string` | `pipeline` (default, v=3) or `agent` (v=2, 一键成片). The agent engine has no mode/theme/language branches; combining them exits 2. |
| `--theme string` | Style ID from `vibeknow theme list --mode <mode>` (must match the mode's catalog; default auto-select) |
| `--language string` | Output language for script + narration: `zh-CN`, `en-US`, `es-ES`, `fr-FR`, `pt-BR`, `ja-JP`, `ko-KR` (default: account locale) |
| `--voice string` | Voice: a `#` or `speech_voice_id` from `vibeknow voice list`; pick one matching `--language` |
| `--avatar string` | Talking-head presenter: `sys_<id>` (public, `vibeknow avatar list`) or `ua_<id>` (your own activated avatar). Not with `--mode handdraw` or `--engine agent` (exit 2). |
| `--avatar-position string` | Corner: `top-left`, `top-right`, `bottom-left`, `bottom-right` (default: saved preference, else top-left) |
| `--avatar-size float` | Circle diameter in px at 1080p base, 120–480 (default: saved preference, else 240) |
| `--aspect string` | `horizontal` (16:9, default) or `vertical` (9:16); `16:9` / `9:16` accepted as aliases |
| `--bgm` | Enable background music (default off) |
| `--pages int` | `--mode image` only: exact page count, 1–20 (image-gen cost scales with it; omit for auto) |
| `--images string` | Mandatory image_index picks from `vibeknow doc images`, e.g. `1,3,5` (not for `--mode replica`) |
| `--kb-id string` | Knowledge base owning `--from <doc_id>` (printed by `vibeknow doc upload`) |
| `--async` | Print task_id/session_id and exit without waiting |
| `--preview-dir string` | Write cover/MP4 artifacts here and announce each as a `resource_ready` event |
| `--confirm string` | `action_id` from a previously blocked `--export`, once the user has agreed to the spend |

**Sync mode (default):**
- TTY: colored progress bar showing current stage, elapsed time, ETA. Prints video URL on completion.
- Non-TTY / `--output ndjson`: NDJSON event stream, one JSON object per line.

**Async mode (`--async`):**
- Waits until the backend confirms the run started (seconds), then detaches and exits 0. A run rejected at submit time — bad input, no credits — exits non-zero here instead of handing back a `task_id` for a run that never began.
- `--output json`: outputs `{"task_id":42,"session_id":"s_yyy","schema_version":"1"}` (`task_id` is a number).
- `--output ndjson`: one `{"event":"task.submitted",...}` line.
- Cannot be combined with `--export`: export only starts once the preview is done, which `--async` does not wait for. The combination exits 2.
- The pair is also written to the local run ledger, so `vibeknow video wait` with no arguments reattaches to it.

### Presets

A preset is a YAML file holding a reusable combination of *style* flags, so a
team that has settled on how its videos look does not re-type a dozen options
on every run.

```yaml
# ~/.config/vibeknow/presets/brand-explainer.yaml
schema_version: "1"
name: brand-explainer
description: how our explainers look
create:
  mode: image
  aspect: horizontal
  language: zh-CN
  bgm: true
  voice: "12"
  pages: 8
  images: [1, 3, 5]     # a list, or the string "1,3,5"
```

```
vibeknow create --from deck.pdf --preset brand-explainer
vibeknow create --from deck.pdf --preset ./team/shorts.yaml
```

`--preset <name>` (no separator, no `.yaml`) is looked up in
`<config>/presets/`; anything else is a path. A name that does not exist
exits 2 and lists the names that do — there is no `preset list` command.

**Precedence.** A preset only supplies defaults. Every flag also given on the
command line wins, including an explicit `--bgm=false`. A preset can never
contradict what you typed.

**Keys.** Underscores and dashes are interchangeable (`script_lock` =
`script-lock`). Allowed: `mode`, `script-lock`, `aspect`, `bgm`, `engine`,
`pages`, `images`, `theme`, `language`, `voice`, `prompt`, `avatar`,
`avatar-position`, `avatar-size`.

**Refused, with the reason named:**

| Key | Why |
|-----|-----|
| `export`, `yes`, `confirm` | Authorise a charge. Consent that arrives inside a file someone forwarded to you is not consent — pass these on the command line. |
| `from`, `kb-id` | Identify one run's input, not a reusable style. |
| `async`, `preview-dir`, `output` | Describe one invocation, not the style. |

Any unknown key, unsupported `schema_version`, empty `create:` block, or
value the flag itself rejects (`pages: many`) exits 2 **before** anything is
uploaded, so a broken preset costs nothing. Nothing is ever silently dropped.

Every run says which preset it applied and which keys took effect: a
`preset.applied` event on the structured channel, or one stderr line in text
mode.

## video status

Get current status of a video task.

```
vibeknow video status <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger; see `vibeknow jobs list`) |

## video wait

Stream progress for a video task, blocking until terminal state (succeeded/failed/cancelled).

```
vibeknow video wait <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger; see `vibeknow jobs list`) |
| `--preview-dir string` | Write the cover still here and announce it as a `resource_ready` event |

Behavior is identical to sync-mode `create` once the task is already submitted. Useful after `create --async` or to recover from exit code 6 (stream interrupted).

Exiting 0 from `wait` always means the task genuinely reached a terminal
state. A stream that closes without one exits 6 and carries the
`resend_safe` verdict — see [errors.md](errors.md#exit-6-errordetail).

## video export

Render the MP4 for a work. Takes several minutes and **costs credits**.

```
vibeknow video export [task_id] [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger, then from the account's most recent run) |
| `--async` | Submit and return; do not wait |
| `--yes`, `-y` | Skip the confirmation gate — only when the user has already authorised this spend |
| `--confirm string` | `action_id` from a previously blocked run |
| `--preview-dir string` | Write the rendered MP4 here and announce it as a `resource_ready` event |
| `--timeout duration` | Local deadline for the sync wait. Does **not** cancel the backend render. |
| `--poll-interval duration` | Fixed poll interval (overrides exponential backoff) |

**The confirmation gate.** With a terminal attached you get a `[y/N]`
prompt. Without one — an agent, CI — the command does not decide for you:
it exits **8** and writes the decision to stdout as `pending_actions`. Show
the user what it costs, then run the action's `resume_command`. See
[errors.md](errors.md#exit-8-blocked-on-a-decision).

**Exit codes.** `0` the MP4 exists; `7` the preview is fine but the render
failed; `6` the local wait was interrupted or timed out (the backend keeps
going — reattach with `vibeknow video export-status`); `8` blocked on the
spend decision.

## video download

Download the rendered video file for a completed task.

```
vibeknow video download <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger; see `vibeknow jobs list`) |
| `--dest string` | Destination file path (default: `<session_id>.mp4`) |
| `--overwrite` | Overwrite existing output file |

**Note:** The file path is `--dest`. Until 0.8 it was `--output`, which shadowed the global format flag; `--output` now means format here as it does on every other command. Passing a path to `--output` fails with exit 2 and a message pointing at `--dest`.

Supports HTTP Range (resume). If download is interrupted, re-run the same command to resume.

## voice list

List voices: public templates grouped by language, plus the signed-in
user's cloned voices (cloned voices are language-independent).

```
vibeknow voice list [flags]
```

| Flag | Description |
|------|-------------|
| `--language string` | Only show public voices for one locale (e.g. `zh-CN`, `en-US`); cloned voices always show |

**JSON output shape:**
```json
{
  "templates": [{"id": 1, "name": "…", "category": "…", "language": "zh-CN", "speech_voice_id": "sv_…"}],
  "languages": [{"language": "zh-CN", "voices": [{"id": 1, "…": "…"}]}],
  "cloned":    [{"id": 9, "name": "…", "speech_voice_id": "sv_…"}]
}
```

`templates` is the flat view (every public voice, catalog order); pass
either a listed `id` or a `speech_voice_id` to `create --voice`.

## theme list

List visual themes/styles for one creation mode. Theme IDs feed
`create --theme` and must come from the catalog of the mode you create
with (the backend rejects cross-mode themes).

```
vibeknow theme list [--mode default|image|handdraw|replica] [flags]
```

`default` and `replica` share one catalog (the design suite); `image` and
`handdraw` each have their own. Hand-drawn themes include `preview` URLs
(webp/poster, horizontal + vertical).

**JSON output shape:**
```json
{"mode": "image", "themes": [{"id": "…", "name": "…", "desc": "…", "tags": ["…"]}]}
```

## avatar list

List talking-head presenters: public presets (`sys_<id>`) merged with the
signed-in user's own trained avatars (`ua_<id>`, all states — only
`active` ones are usable). Public presets carry a paired `voice_id`
(the voice their demo uses); pass it to `create --voice` to keep face and
voice matched.

```
vibeknow avatar list [flags]
```

**JSON output shape:**
```json
{
  "public": [{"id": "sys_7", "name": "…", "style": "3d", "gender": "female", "voice_id": "sv_…"}],
  "mine":   [{"id": "ua_12", "name": "…", "status": "active", "usable": true}]
}
```

## video avatar-retry

Retry a work's **failed** avatar scenes. Failed scenes are terminal and
block `video export` permanently until retried; the retry re-renders only
those scenes (script/images/TTS untouched) and bills nothing.

```
vibeknow video avatar-retry [task_id] [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger) |
| `--scene int` | Retry only this scene index (default: every failed scene) |

`retry_count: 0` in the output means nothing was failed — if export is
still refused, the scenes are *pending* (still rendering); wait instead of
retrying.

## video pause / video resume

Stop a run that is generating, and continue it later — or retry a *failed*
run from its last checkpoint instead of paying for a whole new one.

```
vibeknow video pause  [task_id] [flags]
vibeknow video resume [task_id] [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger) |

Pause keeps the work already done; resume picks up from there rather than
starting over. Reach for pause the moment a run turns out to be wrong —
letting it finish means paying for a video nobody asked for.

`resume` reports which of two things it did:

| `mode` | Meaning |
|--------|---------|
| `paused_resume` | Continued a run the user had paused |
| `failed_checkpoint_retry` | Retried a failed run from its checkpoint; the backend reopens the original bill, so this costs far less than creating the video again |

Three refusals exit **5** and no retry clears them: the run used
`--engine agent` (that line keeps no checkpoint), it was stopped by the
provider's content policy (the same inputs would be stopped again), or it
is neither paused nor failed. Exit **4** (`session busy`) is the one worth
retrying — another pause/resume is in flight on the same session.

## video script

Read what a finished work says, shot by shot. Read-only and free — it
queries stored rows, re-runs nothing, and bills nothing.

```
vibeknow video script [task_id] [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID (default: resolved from the local run ledger) |

Until this existed, everything readable about a finished work described the
container — title, duration, cover, share link. The narration lived only
inside the generation stream, whose progress events carry a character
*count*, so "what does this video say" had no answer once the run ended.

JSON output:

| Field | Meaning |
|-------|---------|
| `script` | The whole narration stitched in shot order — what "show me the script" wants |
| `scenes[]` | Per shot: `scene_index`, `name`, `script_text`, `duration_sec`, `layout_type`, `status` |
| `scenes[].tts_url` / `srt_url` / `bg_image_url` | Narration audio / subtitles / still for that shot; **omitted entirely when absent**, so a missing key means "not rendered yet" rather than a broken link |
| `scene_count` / `duration_sec` | Totals |

Exit **6** means the work has no shots recorded yet — usually still
generating. Check `video status` rather than retrying.

Reading is not editing: there is no command that changes a script, image or
subtitle. Adjusting content still means a fresh `create`, which bills again.

## video set

Change what a finished video *presents* — music, subtitles, title — without
regenerating it. Free and immediate.

```
vibeknow video set [task_id] [flags]
```

| Flag | Description |
|------|-------------|
| `--bgm on\|off` | Background music |
| `--bgm-volume float` | Music level, 0.1–2.0 (1.0 = unchanged) |
| `--subtitle on\|off` | Subtitles |
| `--subtitle-size int` | Subtitle font size in px |
| `--subtitle-color string` | Subtitle colour, e.g. `#FFFFFF` |
| `--subtitle-font string` | Subtitle font family (backend allow-list) |
| `--title string` | Rename the task |
| `--session-id string` | Session ID (default: resolved from the local run ledger) |

Several can be combined in one call. Only the flags you pass are touched:
subtitle style is read, modified and written back, because the backend stores
the style wholesale and a write-only change would clear the font, weight,
colour, outline and animation along with whatever was being adjusted.

**Every change except `--title` discards the rendered MP4.** The change has to
be baked into the file, so the old one no longer matches the work. Preview and
share link keep working; the download does not, until the next `video export`
— which bills and asks for confirmation. The response reports this as
`export_invalidated`, with a `next_actions` entry naming the export command.

Exit **5** means this work cannot take that setting (an engine without BGM
volume, a renderer that cannot carry styled subtitles). No value fixes it.

This changes presentation only. Narration, images and layout have no edit
command — those still require a fresh `create`, at full price.
