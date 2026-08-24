# vibeknow-create Command Reference

## Global Flags

| Flag | Description |
|------|-------------|
| `--output string` | Output format: `text\|json\|ndjson` (default `text`; `VIBEKNOW_OUTPUT` sets the default, an explicit flag wins). An unrecognized value exits 2 rather than falling back to text. |
| `--profile string` | Override active profile for this command |
| `--verbose` | Emit request/response summaries (credentials redacted) |

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

Behavior is identical to sync-mode `create` once the task is already submitted. Useful after `create --async` or to recover from exit code 6 (stream interrupted).

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
