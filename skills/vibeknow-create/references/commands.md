# vibeknow-create Command Reference

## Global Flags

| Flag | Description |
|------|-------------|
| `--output string` | Output format: `text\|json\|ndjson` (auto-selects based on TTY) |
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
- Prints `task_id` and `session_id`, then exits immediately (exit 0).
- `--output json`: outputs `{"event":"task.submitted","task_id":"...","session_id":"...","schema_version":"1"}`.

## video status

Get current status of a video task.

```
vibeknow video status <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID **(required)** |

## video wait

Stream progress for a video task, blocking until terminal state (succeeded/failed/cancelled).

```
vibeknow video wait <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID **(required)** |

Behavior is identical to sync-mode `create` once the task is already submitted. Useful after `create --async` or to recover from exit code 6 (stream interrupted).

## video download

Download the rendered video file for a completed task.

```
vibeknow video download <task_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--session-id string` | Session ID **(required)** |
| `--output string` | Output file path (default: `<session_id>.mp4`) |
| `--overwrite` | Overwrite existing output file |

**Note:** The `--output` flag on `download` sets the file path, NOT the output format. The global `--output` (format) flag is not available on this subcommand.

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
