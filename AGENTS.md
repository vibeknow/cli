# Agent guidance — vibeknow-cli repo

This repo is a Cobra-based Go CLI with an npm distribution wrapper.

## Where to look
- `cmd/` — Cobra command definitions, one file per command.
- `internal/` — Reusable framework packages (output, i18n, credential, config).
- `docs/superpowers/specs/` — Design documents; `docs/superpowers/plans/` — Implementation plans.
- `skills/` — AI agent Skills (empty in P0).

## Conventions
- All user-visible strings go through `internal/i18n`.
- Tests live alongside source (`*_test.go`) plus `tests/integration/`.
- `--output` values: `text` (default in TTY), `json`, `ndjson`.
- Exit codes: see spec §5.4.

## Before making changes
- Run `make test` and `make build`.
- Never commit credentials, cache files (`~/.cache/vibeknow/*`), or `dist/`.

## Two-stage video workflow

The video pipeline has two stages — agents should understand both:

1. **Preview** (produced by `vk create`): a shareable HTML page. The
   `share_url` in the output is the primary artifact; it plays the
   finished video.
2. **Export** (optional, via `vk video export`): renders a downloadable
   MP4. Takes several minutes and extra credits.

Commands:
- `vk create --from <src>` — pipeline → preview → `share_url`
- `vk video status [task_id] [--session-id <sess>] --output json` —
  full snapshot: `{preview, export, next_actions}`. Agents should
  follow `next_actions[].command` to plan the next step.
- `vk video export [task_id] [--session-id <sess>] --yes --output json` —
  render MP4. `--yes` skips the confirmation prompt; `VIBEKNOW_ASSUME_YES=1`
  works at the environment level.
- `vk video download [task_id] [--session-id <sess>] --dest out.mp4` —
  pure download (requires export to have completed). The file path is
  `--dest`; `--output` means format here as it does everywhere else.

`--session-id` is optional on all four: when omitted it is resolved from
the local run ledger (see below). An explicit value always wins.

Exit codes: `0` success, `2` user error, `3` auth, `4` retryable, `5`
business failure, `6` interrupted / state unknown, `7` partial success
(preview ready, export failed).

The contract holds on **every** command, not just `vk create`:

- Anything wrong with the command line — unknown command, unknown or
  misspelled flag, missing required flag, stray positional argument, a
  bad enum value — exits **2**. An unknown flag close to a real one
  names the real one in the hint.
- An expired or missing credential exits **3** on every command that
  talks to a backend. Re-authenticate and retry.
- `rate_limited`, `internal_error` and `concurrent_work_limit` exit
  **4**: the same command is worth retrying after a wait.
- `insufficient_credits` exits **5**: retrying cannot help.

Nothing exits 0 with output the caller cannot act on. A malformed
command never prints help to stdout and calls it success.

**Which commands adopt a failure as their exit code.** A command that
*blocks until a terminal state* takes its exit code from that state:
`vk create`, `vk video wait`, and `vk video export` (without `--async`)
all do. A command that takes a *single-shot reading* exits 0 and reports
the state in its payload: `vk video status`, `vk video export-status`.
So a failed render is exit 7 from `export` and exit 0 +
`export.status="failed"` from `export-status`. Neither ever exits 0 with
no way to tell the outcome.

## Run ledger (`vk jobs`)

Every run is addressed by a `(task_id, session_id)` pair. `vk create`
records that pair in `<config-dir>/jobs.jsonl` so a caller that lost it
— context trimmed, terminal closed, process restarted — can still reach
a run that is live and billing.

- `vk jobs list [--active] [--output json]` — recorded runs, newest first
- `vk jobs get [task_id]` — one run (default: the most recent)
- `vk jobs prune --terminal | --older-than 30d | --all` — forget entries.
  A bare `prune` is refused; it would drop the pointer to a live run.

The ledger is local and advisory — the backend owns run state. A missing
entry means "not recorded here", never "does not exist", and every video
subcommand still accepts an explicit `--session-id`. Ledger writes are
best-effort: a failure warns on stderr and never fails the run.

## Async submission

`vk create --async` submits the run and returns without waiting for the
render. It does **not** skip starting the pipeline: it still opens the
generation request (that request, not task init, is what starts the
backend run), waits only until the backend emits its first event, and
then detaches. The run continues server-side; the CLI disconnecting
does not stop it.

Consequences worth planning around:

- `--async` returns in seconds, not instantly. If the backend rejects
  the run (bad input, no credits), you get the failure **here**, with
  the same exit code the synchronous path would have used — not a
  task_id for a run that was never going to happen.
- On success it prints `task_id` / `session_id`, honouring `--output`:
  `json` → `{"task_id":…,"session_id":…}`, `ndjson` → a
  `task.submitted` event.
- If the backend accepts the request but reports no progress within 60s,
  the CLI detaches anyway and says so on stderr. Treat that as
  unconfirmed and verify with `vk video wait` before relying on it.
- `--async` and `--export` cannot be combined and are rejected with exit
  2. Export can only start once the preview is finished, which `--async`
  by definition does not wait for. Submit, `vk video wait`, then export.

`vk video wait [task_id]` reattaches to a running task and streams its
events. It exits **6**, not 0, when the stream closes without a terminal
event — including the case where there is nothing to observe at all,
which means the task was never dispatched. An exit 0 from `wait` always
means the task genuinely finished.

## Output format

`--output text|json|ndjson` is a persistent flag on every command, and
every command honours it — including `doc upload`, `credits balance`,
`doctor`, `version`, `whoami`, and the config/profile families.

- Default is `text`. There is **no** TTY-based auto-switching; format is
  always explicit, as in `gh` and `kubectl`.
- `VIBEKNOW_OUTPUT` sets the default for callers that do not want to
  thread the flag through every invocation. Precedence: `--output` >
  `VIBEKNOW_OUTPUT` > `text`.
- An unrecognized value is a validation error (exit 2), not a silent
  fall-through to text.
- stdout carries data, stderr carries everything else (progress,
  warnings, hints). `vk doc upload x.pdf --output json | jq -r .doc_id`
  works.

## Video kinds

`vk create --mode <kind>` picks which figlens pipeline runs:

- **default (no flag)** — generative video from the document.
- **`--mode replica`** — PPT/PDF page-by-page reproduction. Suited
  for slide decks where the visual structure is the message.
- **`--mode image`** — one AI-generated illustration per narration
  page. `--pages N` sets the page count (image-gen cost scales with
  it); `--pages` is rejected outside this mode.
- **`--mode handdraw`** — hand-drawn animation. Runs its own graph
  (illustration → vectorization), so it is pipeline-only.

## Script lock

`--script-lock` uses the uploaded document text *verbatim* as the
narration instead of writing a script from it. It is **orthogonal to
`--mode`** — it rides on top of whichever line was picked, so
`--mode image --script-lock` illustrates the user's own script.

The doc must pass a quality preflight (length, characters, content)
which runs at task-init time, before any billing. Preflight failures
exit **2** with a clear message; agents should treat them as
user-input problems, not retries. The preflight only fires when the
kb/doc pair is known, so `--script-lock` requires `--from <file|URL>`
or `--from <doc_id> --kb-id <kb>`.

`--mode script` is a **deprecated alias** for `--script-lock` and
prints a warning. It dates from when the backend expressed the lock
as a `video_kind` value rather than a separate boolean; the two axes
have since been split, and the alias now resolves to the boolean.

Modes combine freely with `--aspect horizontal|vertical` (or `16:9` /
`9:16`) and `--bgm`. All these flags are independent of `--from`,
`--prompt`, `--voice`, and `--export`.

Exit-code summary for new modes:
- `2` — `--mode <bad>`, `--aspect <bad>`, or script preflight rejected the document.
- `5` — pipeline business failure (e.g., insufficient credits) — same as today.
- `7` — preview ok, MP4 export failed — same as today.

## Engine selection

`vk create --engine <name>` picks which go-figlens engine generates the
video. Two engines exist and are both actively maintained on the
backend:

- **`--engine pipeline`** (default) — v=3, graph-based pipeline. Has
  rich SSE node events (`[parse] prepare started`, `[outline]
  text_speech done`, ...). All `--mode` values supported.
- **`--engine agent`** — v=2, agent-driven flow. SSE events are
  free-form progress messages without a node graph; CLI shows them as
  `[agent] <message>` in text mode or `node.progress` in NDJSON.
  Supports `--script-lock` but **none** of `--mode
  replica|image|handdraw` — each of those runs a dedicated graph the
  agent engine never dispatches to, so the CLI rejects the
  combination with exit 2 rather than letting the backend silently
  render an ordinary video instead.

The `engine` field in JSON snapshot output reflects which engine ran
(`"pipeline"` / `"agent"`), letting agents verify routing.

## Voice selection

`vk voice list` prints two identifiers per row. `SPEECH_VOICE_ID` is the
one the backend's TTS keys on; the leading `#` is a display index.
`--voice` accepts either — a `#` is translated to its speech_voice_id
before anything is uploaded, and a `#` that is not in the list exits **2**
straight away rather than failing inside the TTS node after cover and
background images have already been billed.

Non-numeric values pass through unvalidated, because cloned voices do
not appear in the template list.

## KB management

`vk kb list / delete / prune` manage vectoria knowledgebases. The
headline command is `vk kb prune`, which exists to clean up the kb
backlog that `vk create` accumulates (one new kb per invocation).

Safety contract for `vk kb prune`:

- **Refuses to run without `--pattern` or `--older-than`** (exit 2).
  No "delete all" shortcut.
- **Dry-run by default**: prints matched kbs without issuing any
  DELETE. `--yes` (or `VIBEKNOW_ASSUME_YES=1`) actually deletes.
- **Idempotent**: a 404 on DELETE counts as success (kb already gone).
- **Partial failure**: exit 7 if some succeed and some fail; exit 5
  if all fail; exit 0 if all succeed or matched empty.

Recipes:

- Clean CLI's own orphans: `vk kb prune --pattern 'vibeknow-cli-*' --yes`
- Clean old kbs (last 30 days): `vk kb prune --older-than 30d --yes`
- Both: `vk kb prune --pattern '*' --older-than 90d --yes`

Output via `--output json` for piping; default text mode is
human-friendly.
