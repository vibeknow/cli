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
| `--voice string` | Voice template ID (use `vibeknow voice list` to browse) |
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

List available voice templates.

```
vibeknow voice list [flags]
```

No command-specific flags.

**JSON output example:**
```json
[{"id":"v_warm_female","name":"Warm Female","language":"en","preview_url":"https://..."}]
```
