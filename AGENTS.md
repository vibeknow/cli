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
(preview ready, export failed), `8` blocked on a user decision.

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
- A paid action reached with no terminal attached and no prior authority
  exits **8**: the decision was handed back, not made.

Nothing exits 0 with output the caller cannot act on. A malformed
command never prints help to stdout and calls it success.

**Exit 6 says whether a resend is safe.** "State unknown" is true and
useless on its own: the caller's real question is whether re-running
recovers a lost run or pays for a second one. Before returning 6 the CLI
reads the work row and puts the verdict in `error.detail`:

| `delivery` | `resend_safe` | Basis |
|------------|---------------|-------|
| `submitted` | false | the backend has a work row for this session |
| `not_submitted` | true | the backend returns not_found — the work row is created at init, so its absence means nothing was billed |
| `indeterminate` | false | the probe itself failed; unknown is not permission |

`indeterminate` deliberately reports `resend_safe: false`. Waiting when a
run was already lost costs a delay; resending when it was not costs the
user's money.

## Spend decisions (`exit 8`)

A confirmation prompt assumes someone is there to answer it. When an agent
runs the CLI nobody is, and both available answers are bad: block forever,
or spend the user's credits and mention it afterwards. The CLI used to do
the second.

`cmdutil.Gate` resolves a paid action in this order:

1. `--yes` / `VIBEKNOW_ASSUME_YES` — the caller states it has authority.
2. `--confirm <action_id>` — authority obtained through this gate.
3. A terminal — ask, the way a person expects to be asked.
4. Otherwise — write the decision to **stdout** and exit **8**.

The blocked payload carries `pending_actions[]` with an opaque `action_id`,
the `payload` being agreed to, `options[].effect` (`resume` / `none`), and
a `resume_command` spelled out in full.

`action_id` is an HMAC over the action type and the decision-relevant
payload, keyed by a per-installation secret in `<config-dir>/action.key`.
Because the payload is hashed and not merely displayed, a token stops
verifying when the terms change — consent is to a specific price for a
specific run, and a token that survived a price change would be consent to
a number the user never saw.

This is **not** a security boundary; anything running as the user can read
the key. It is an evidence boundary: the token cannot be produced by
reasoning about the conversation, so a model that has one went through the
code path that disclosed the cost. The failure being prevented is confident
invention, not a determined bypass.

Exit 8 rather than 0 (which is what Flova, the design this borrows from,
returns here) because our contract puts the outcome in the exit code
precisely so a small model does not have to parse anything. `vk video
export` exiting 0 states that the MP4 exists; at this boundary it does not.
Not 2 either — the command line is fine, and sending an agent to look for
an argument mistake it did not make wastes a turn.

Scoped to the two paid paths (`vk video export`, `vk create --export`).
`kb delete` keeps the plain prompt: it is destructive but not billed, and
`kb prune`, the bulk path, is already dry-run by default.

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

Because it is per-machine, "not recorded here" is a weak answer: an agent
that moved hosts, a rebuilt container, or a colleague picking the work up
all have a run reachable through the account and no local file mentioning
it. So `resolveTarget` falls back to the backend's most recent work when
the ledger has nothing, and says so on stderr — the caller must be able to
tell which run it got.

The fallback only covers the no-arguments case. A `task_id` cannot be
resolved remotely: the backend's work list is keyed by `work_id` and
`session_id` and carries no `task_id`, so `vk video status 12345` with an
empty ledger exits 2 pointing at `vk video list`, rather than pretending to
look something up it cannot.

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

### Structured progress on stderr

`--output json` used to be silent until the end, and `--output ndjson` put
the progress stream on stdout where it displaced the final answer — a
consumer had to read the whole stream and work out for itself which line
was terminal. Choosing between watching a run and parsing its result was
not a choice worth making.

Long-running commands now write progress to **stderr** as
`vk_event={"schema_version":"1","ts":…,"type":…}` lines while stdout keeps
carrying exactly one document. The prefix is what makes it possible for the
machine lines to share a stream with human text: a consumer picks them out
by string match instead of attempting a JSON parse of every line.

- On by default when `--output json`; `VIBEKNOW_EVENTS=1` / `=0` overrides.
- Off in `text` (stderr is a person's progress display) and in `ndjson`
  (the stdout stream is released; duplicating it would double every event).
- The payload is `StreamEvent.NDJSONFields()` — the same shape as the
  stdout stream, so a consumer needs one parser, not two.
- When the channel is on, the command skips the human prose it would
  otherwise write for the same event. stderr never carries both renderings
  of one fact.

## Preview delivery (`--preview-dir`)

`share_url` is a hosted page. An agent driving the CLI from a terminal
cannot open one, so the single output of a video tool worth looking at was
the one output an agent could not pass on.

`--preview-dir <dir>` on `create`, `video wait` and `video export` writes
the run's artifacts there and announces each as a `resource_ready` event on
the structured channel:

```
vk_event={"type":"resource_ready","asset_kind":"cover_image",
          "session_id":"s_x","local_path":"/abs/path/s_x-cover_image.jpg","bytes":48210}
```

- `asset_kind` is `cover_image` or `video_playback`.
- `local_path` is absolute and the file is fully written before the event
  fires — download goes to a temp file in the same directory and is renamed
  last, so a reader never sees a partial one.
- The source URL is **never** in the event. Those are signed; an agent that
  relays one has published a credential.
- Dedupe is by content hash against what is already at the destination, not
  by URL — the backend re-signs unchanged assets, so keying on the address
  would re-deliver the same still on every call. Because the comparison is
  against the file on disk, it survives across processes: re-running into
  the same directory is silent.
- Failures emit `resource_preview_warning` and never fail the run. The
  video rendered; a missing thumbnail is not evidence about that.

## Presets (`--preset`)

`vk create` takes 21 flags, and most of them describe a *style* rather than
a run — mode, aspect, theme, voice, language, bgm, avatar placement. A
preset is that combination in a version-controllable YAML file:

```yaml
schema_version: "1"
name: brand-explainer
create:
  mode: image
  aspect: horizontal
  language: zh-CN
  bgm: true
```

`--preset <name>` resolves under `<config>/presets/`; anything with a path
separator or a `.yaml`/`.yml` extension is a path. Keys are flag names;
underscores and dashes are interchangeable.

Two rules make it safe to run a file someone else wrote.

**A preset only supplies defaults.** `Apply` skips every flag cobra reports
as `Changed`, so the command line always wins — including an explicit
`--bgm=false`. A file can never contradict what the caller typed.

**The option set is an allowlist.** A preset cannot set `--export`, `--yes`
or `--confirm`: those authorize a charge, and consent that arrives inside a
forwarded file is not consent. It cannot set `--from`/`--kb-id` (one run's
input, not a style) or `--async`/`--preview-dir`/`--output` (one
invocation, not a style). Each refusal is an explicit exit 2 naming the key
and the reason — a `yes: true` that were silently dropped would read as
though it had worked, which is the failure mode this whole contract exists
to remove.

Expansion happens at the top of `RunE`, before the first upload, so every
rejection is free and everything downstream — validation included — sees
one flag set and cannot tell a preset value from a typed one. Values go
through `pflag.Set`, so `pages: many` fails with the same message the
command line gives.

Every run reports what it applied: a `preset.applied` event carrying the
sorted list of keys that actually took effect, or one stderr line in text
mode.

This is deliberately client-side only. It is **not** a place for per-node
instructions: the task init request has no field to carry them
(`client/figlens/task.go:31-56`), so a preset can express exactly what the
command line can express and nothing more.

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
