# Changelog

## Unreleased

### Fixed — the exit-code contract now holds on every command

Found by driving all 37 commands through a stub backend across output
formats, error paths and backend failure modes.

- **A malformed command line exits 2, not 1.** Unknown commands, unknown
  or misspelled flags, missing required flags, stray positional
  arguments — the mistakes a caller makes most often, and the most
  trivially correctable — all reported as generic failures. A caller
  branching on the exit code could not tell "your arguments are wrong"
  from "the backend broke".
- **An unknown flag now names the closest real one.** `--kbid` →
  "did you mean --kb-id?".
- **`vk <group> <typo>` exited 0 and printed help to stdout.** Cobra
  short-circuits a non-runnable command to help without validating args,
  so a typo'd subcommand looked like success and put help text where a
  caller expected data. Group commands validate first now; a bare
  `vk video` still prints help.
- **Commands that take no positional arguments no longer ignore them.**
  `vk version extra` silently discarded `extra` and exited 0.
- **An expired credential exits 3 on every command.** Only `vk create`
  translated backend codes into exit codes; everything else exited 1, so
  the documented "exit 3 → re-authenticate" instruction never fired for
  `credits balance`, `video list`, `kb list` and the rest. The same
  central mapping gives `rate_limited` / `internal_error` /
  `concurrent_work_limit` exit 4 and `insufficient_credits` exit 5
  everywhere.
- **`vk create` exited 0 with empty stdout when the stream closed
  without a terminal event.** The reattach path (`video wait`) was fixed
  earlier in this release; the primary path still reported success for a
  run whose state was unknown and which may well still have been
  running. It exits 6 now, records the run as unresolved, and prints the
  `video wait` command that reattaches.
- **Progress from an unrecognized pipeline node was dropped.** The CLI
  skipped node events whose `step_id` this build does not know, so it
  went silent for the whole of that node — on an operation that runs for
  minutes, against a backend that has already renamed its graph once.
  They are forwarded as free-form progress instead, without the raw wire
  name (which may carry an internal codename).

### Changed — fewer ways to get it wrong

- `--size` and `--limit` are accepted interchangeably on the three list
  commands, and `--no-wait` works wherever `--async` does. `--help`
  still shows one name per concept.
- `create --async --output json` now carries `next_actions`, like every
  other JSON payload. It was the one command that handed back two bare
  ids and left the caller to infer the follow-up.
- A confirmation auto-accepted because there is no TTY now says so on
  stderr. Proceeding is still right — an agent cannot answer a prompt —
  but some of these prompts gate billed operations, and doing it in
  silence left no trace that a gate had been skipped.

### Added — run ledger (`vk jobs`)

- Every run is addressed by a `(task_id, session_id)` pair, and the CLI
  used to print it once and forget it. `--session-id` was mandatory on
  every `video` subcommand, so a caller that lost that string — an agent
  whose context was trimmed, a closed terminal, a restarted process —
  could not reach a run that was live and still billing. The only way
  forward was `vk create` again: a second render, a second charge.

  `vk create` now records the pair in `<config-dir>/jobs.jsonl`, and
  `--session-id` became optional everywhere it was required:

  ```
  vk video wait          # reattach to the most recent run
  vk video wait 42       # session_id looked up for task 42
  vk jobs list --active  # runs that have not reached a terminal state
  ```

  An explicit `--session-id` always wins, so a stale entry can never
  override the caller. The ledger is local and advisory — the backend
  owns run state, a missing entry means "not recorded here". Writes are
  best-effort: a failure warns on stderr and never fails the run it was
  describing.

  `vk jobs list|get|prune` inspect it. A bare `prune` is refused: it
  would drop the pointer to a run still in flight, which is not
  recoverable from the CLI. The file is append-only so concurrent
  `vk create` processes need no lock, and it compacts itself.

### Fixed — commands that reported failure as success

- `vk video export` exited **0** when the render failed. `PollExport`
  treats "the backend reported failure" as a successful poll and returns
  the failed result with a nil error, and the command passed that
  straight through: `export.status: "failed"` in the payload, exit 0 on
  the process. An agent branching on the exit code went on to
  `vk video download` an MP4 that was never produced. It now exits **7**,
  matching the `vk create --export` chain.

  The rule is now stated in AGENTS.md: a command that *blocks until a
  terminal state* (`create`, `video wait`, `video export`) takes its exit
  code from that state; a *single-shot query* (`video status`,
  `video export-status`) exits 0 and reports the state in its payload.

- `create --async --export` silently dropped `--export` and exited 0.
  Export runs after the preview snapshot, which `--async` never reaches
  — so a caller asking for an MP4 got a preview and no signal. The
  combination now exits **2** before any network call, with the two-step
  sequence that does work.

- `AuthMiddleware` discarded errors from `Token()` and sent the request
  unauthenticated. "No credential found", "login expired" and
  "session replaced on another device" all became an opaque backend 401,
  losing the instruction that would have fixed them. They now surface as
  exit **3** with the provider's own message; the underlying structured
  error stays reachable through `errors.As`.

- `vk api call` exited 1 on 401/403, making an expired login
  indistinguishable from a bad path. It now exits **3**. Its token
  provider also no longer swallows resolver errors.

### Changed — output contract

- 19 of 33 commands accepted `--output json`, ignored it, and printed
  human prose on stdout at exit 0. A caller piping into `jq` got a parse
  error with no way to tell a broken command from an unimplemented flag.
  Every command now honours it, including `doc upload`,
  `credits balance`, `doctor`, `version`, `auth whoami|logout|login`, and
  the whole `config` / `profile` family.

- `--output` help claimed it "auto-selects based on TTY". It never did.
  The text is corrected rather than the behaviour implemented: auto-
  switching would change the output of every existing pipeline on
  upgrade, and `gh`, `kubectl` and `docker` all keep the format explicit.
  `VIBEKNOW_OUTPUT` is the middle path — it sets the default for
  non-interactive callers, and an explicit `--output` still wins.

- An unrecognized `--output` value is now a validation error (exit 2).
  `--output jsonl` used to print prose and exit 0.

- **Breaking:** `vk video download --output <path>` is now
  `--dest <path>`. The local flag shadowed the global format flag, so
  `vk video download --output json` wrote a file literally named "json".
  Passing a path to `--output` fails with exit 2 and a message naming
  `--dest`.

- `vk doc upload` moves its progress narration to stderr, so
  `vk doc upload x.pdf --output json | jq -r .doc_id` works. `vk doctor`
  renders its report before returning the failure error, so
  `--output json` still gets the report when checks fail — precisely when
  it is wanted.

- `vk video download` binds its HTTP GET to the command context, so
  Ctrl-C interrupts a large download.

### Fixed (`--async` never started the task — #10)

- `create --async` returned right after `tasks/init`, never issuing the
  generation request. But init only reserves the task/session/work rows;
  the *stream* request is what dispatches the pipeline. Every `--async`
  task was therefore a zombie: stamped "generating" by init, never picked
  up by anything, still at 0% hours later. `video wait` then found no
  events to replay and exited **0** with no output — a silent success for
  a task that never ran, which is the worst possible signal for the agent
  callers this mode exists for.

  `--async` now issues the generation request like the synchronous path
  and detaches once the backend emits its first event, proving the run is
  live. The run survives the disconnect: the backend derives its run
  context from `context.Background()` on its own goroutine, so it is not
  tied to the HTTP request. A rejection that arrives before the first
  progress event (bad input, no credits) is reported by `--async` itself
  with the same exit code the synchronous path uses, instead of handing
  back a task_id for a doomed run. If the backend accepts the request but
  stays silent for 60s, the CLI detaches anyway and says so rather than
  implying the run is confirmed.

- `video wait` no longer exits 0 when the stream closes without a
  terminal event. It exits **6** (task state unknown) and distinguishes
  "no events at all — the task was never dispatched, or the session-id
  does not match" from "stream ended mid-run", each with the check to run
  next.

- `create --async` now honours `--output`: `json` emits
  `{"task_id":…,"session_id":…}` and `ndjson` a `task.submitted` event.
  Previously it printed `task_id=…` as bare text regardless of format,
  forcing agent callers to scrape it.

### Fixed (`--voice` accepted a number that could never work — #12)

- `vk voice list` heads two identifier columns, and only `SPEECH_VOICE_ID`
  works. Passing the other one was accepted silently and blew up minutes
  later inside the TTS node with `40401 音色不存在` — after cover and
  background images had been generated and billed.

  `--voice` now takes either: a list reference number is translated to its
  speech_voice_id before anything is uploaded, and one that is not in the
  list exits **2** immediately. Non-numeric values still pass through
  unvalidated, since cloned voices are absent from the template list. The
  list's numeric column is now headed `#` rather than `ID`, and the flag
  help names both forms.

### Fixed (`--from <doc_id>` failed late with a backend message — #11)

- The backend only accepts a doc_id together with its knowledge_id, but
  the CLI let a bare doc_id through to the stream, where it failed with
  the raw `knowledge_id and doc_id must be provided together` after a
  task, session and work row already existed. `--kb-id` (0.7.1) supplied
  the missing half but nothing pointed users to it — `create --help` still
  advertised doc_id as usable on its own.

  A bare doc_id now exits **2** before any network call, naming `--kb-id`
  and where to find the value. There is no client-side reverse lookup to
  offer instead: every vectoria document call is itself scoped by kb.

### Fixed (script lock was silently a no-op)

- `--mode script` sent `video_kind: "script_lock"`, which the backend
  stopped honouring when it split 原稿锁定 out of the `video_kind` enum
  into an orthogonal `script_lock` boolean. Nothing errored: the value
  matched no pipeline graph, so the request fell through to the standard
  line with the lock off — the user's own script was quietly demoted to
  reference material and rewritten, and the script-quality preflight
  (which is gated on the boolean alone) never ran. Only `--engine agent`
  still worked, since the v=2 line kept reading the string.

  `script_lock` is now a real field on both `tasks/init` and the stream
  request, driven by a new **`--script-lock`** flag that composes with
  every `--mode` (e.g. `--mode image --script-lock` illustrates the
  user's verbatim script). `--mode script` is kept as a deprecated alias
  that resolves to the boolean and prints a warning, so existing scripts
  keep working — and now actually do what they always claimed to.

  Note the prompt-optimize endpoint is unaffected and still keys on
  `video_kind: "script_lock"`; there the value only selects a fixed
  prompt to display, so the CLI flattens the two axes back for that one
  call.

### Added

- `vk create --mode handdraw` runs the backend's hand-drawn animation
  line (illustration → vectorization), the one production mode the CLI
  could not reach. Pipeline engine only.
- `--engine agent` now also rejects `--mode image` and `--mode handdraw`
  alongside `--mode replica`. All three run dedicated graphs the agent
  engine never dispatches to, so the combination previously produced an
  ordinary video with no indication the requested mode was dropped.

## 0.7.1 — 2026-06-11

### Added (image 讲稿生图 mode + mandatory images)

- `vk create --mode image` targets the backend's new AI-illustration line:
  a script is written from the doc, a visual style is vector-matched, and
  each page gets a generated image. Pipeline engine only — `--engine agent`
  is rejected client-side, mirroring the backend.
- `--pages N` (image mode only) pins the exact page count; image-generation
  cost scales with it, `0` lets the storyboard decide. Validated client-side
  so `--pages` without `--mode image` fails fast instead of being silently
  ignored by the backend.
- `vk doc images <doc_id> --kb-id <kb>` lists a parsed document's candidate
  images (`POST /v1/task/extractDocImages`, idempotent) with their
  `image_index` values; `vk create --images 1,3,5` passes the picked
  indexes as `selected_image_indexes` on both `tasks/init` and the stream.
  Rejected client-side for `--mode replica` and `--engine agent`, matching
  backend constraints.
- `vk create --kb-id` restores the kb half of the binding when `--from` is
  a bare doc_id (the backend requires knowledge_id/doc_id as a pair).
- `node.warning` stream events: the backend reports non-fatal degradations
  (e.g. image mode falling back to a placeholder for a failed page) as
  `process` logs with `status=warning`; the CLI previously dropped them.
  Now rendered as `[node] warning: …` in text mode and emitted on NDJSON.

### Changed (mode preflights now fire at init)

- For `--mode replica|script|image` (and whenever `--images` is set) the
  CLI sends the kb/doc pair on `tasks/init`, so the backend's preflights
  (script quality, replica PPT check, doc-support check) reject bad input
  before any credits are spent. New business codes mapped: `100005 →
  replica_invalid`, `100006 → knowledge_unsupported`; both exit 2
  (user-fixable input), and the backend's localized message is printed
  verbatim — previously they fell through as generic `business_error`.
- Stage map covers the replica line's new `doc_dissect` node and the image
  mode's nodes (shown as `style_select`, `image_storyboard`, `image_gen`;
  wire step_ids are sanitized before display).

### Fixed (pipeline creates lost `video_url`/`duration_ms` after the figlens assistant_event rework)

- The figlens update restructured SSE terminal events: the aim_result
  payload now nests under `answer_done` (`answer_done.html_path` carries
  the playable URL on v=3, `answer_done.text` on v=2,
  `answer_done.data.duration_ms` the duration) and failure details nest
  under `error.message`. The CLI still parsed the legacy flat
  `html_path`/`text`/`data`/`message` fields, so `task.succeeded` NDJSON
  events shipped with no `video_url` and no `duration_ms` — downstream
  consumers (e.g. the OpenClaw plugin's `video_url` push) read that as
  "no video was generated" — and `task.failed` dumped raw JSON instead
  of the backend's failure message.
- `client/figlens/stream.go` now parses both shapes: nested
  `answer_done`/`error` first, legacy flat fields as fallback. The
  `text`→URL fallback is gated on an `http(s)://` prefix because on v=3
  the nested `text` is a human-readable completion message, not a URL.
- `internal/stage`: the pipeline graph rework renamed mid-graph nodes
  (`script_writing`/`video_director`/`storyboard_plan`/`scene_filling`
  replaced `text_speech`/`content_analyze`/`design`/`scene_generate`).
  The CLI's known-node filter silently dropped the new IDs, leaving long
  progress gaps that read as a hung generation. Both generations are now
  mapped; old IDs stay for older deployments.
- Adds an integration test pinning the current backend wire format
  end-to-end: `event: data` frames, `retry:` preamble, heartbeat comment
  lines, nested `answer_done`, stream close after `session_completed`
  with no `[DONE]` sentinel.

## 0.7.0 — 2026-05-20

### Changed (default endpoints now point at the production cluster)

- `CloudDefaults` (account / vectoria / figlens / vibeknow / share) now
  resolve to `https://vibeknow.com/<service>` for fresh installs. Existing
  profiles with explicit `endpoints:` overrides in `profiles.yaml` are
  unaffected. Tokens issued against earlier defaults will not authenticate
  against the new endpoints; re-run `vibeknow auth login` (or
  `vibeknow init`) after upgrading.

### Fixed (`preview.ready` reported true while the pipeline was still running)

- The backend generates `Work.ShareToken` at task-submit time, not at
  completion. The previous `Preview.Ready = ShareToken != ""` heuristic
  was therefore true from the moment a task was created — the CLI happily
  reported `preview.ready: true` with a share URL that served a 404 page
  until rendering actually completed.
- Switch to the backend's `Work.Status` enum (already present in
  `WorkResponse` but not parsed by the CLI's `Work` struct). Use
  `Status == Active && ShareToken != ""` defensively. Adds
  `WorkStatus{Generating,Active,Deleted,Failed}` constants on
  `client/figlens/work.go`, in lockstep with the figlens backend's
  WorkStatus enum. Replaces the 0/1/2/3 magic numbers in
  `cmd/video/list.go` with the same constants.

### Fixed (`vibeknow doctor` false-fails against partially-degraded services)

- `probeHealth` now probes `/healthz` first and falls back to `/health`
  on 404, restoring the probe-chain originally documented in 0.3.2 but
  silently dropped from code.
- HTTP 503 with `pillars.databases.status == "healthy"` is now reported
  as `[warn]` (degraded — non-critical pillar down) rather than `[fail]`.
  The exit code is unaffected by warns. Pin the canonical example: an
  account service with a degraded email subsystem is operationally
  usable (SMS-only login flows) and should not break `doctor`.
- HTTP 200 is sufficient to mark a probe healthy regardless of the body's
  `status` string. Different services report `"healthy"` vs `"ok"` vs
  similar; coupling the CLI to a specific keyword caused vectoria
  (`status: "ok"`) to false-fail.

## 0.6.3 — 2026-05-15

### Fixed (NDJSON terminal events were missing result data)

- `task.succeeded` events emitted by `vk create --output ndjson` and
  `vk video wait --output ndjson` now include `video_url` (and, on the
  pipeline engine, `duration_ms`). Previously the CLI dropped these
  fields from the backend `aim_result` SSE payload and forwarded only
  `session_id`, so any NDJSON consumer (agent or shell script) had no
  way to retrieve the rendered video URL from the terminal event
  without making a separate `vk video status` follow-up call.
- `task.failed` events now include a `retryable` boolean derived on
  the CLI side from the error code (transient codes — `rate_limited`,
  `internal_error`, `concurrent_work_limit` — map to `retryable=true`;
  permanent codes like `insufficient_credits` and `script_invalid`
  map to `retryable=false`). The backend's terminal `error` event
  carries no retryable flag of its own, so the CLI is the source of
  truth for this signal.
- `vk create` exit codes now honor the `retryable` flag: transient
  task failures exit **4** (previously always 5). `vk video wait`
  gained the same exit-code mapping plus `script_invalid → exit 2`
  parity with `vk create`. Shell scripts that previously hard-coded
  `[ $? = 5 ]` to detect any task failure should switch to
  `[ $? -ge 4 ] && [ $? -le 5 ]` or branch on the NDJSON `retryable`
  field directly.
- HTTP `/v1/tasks/init` errors now share the same retryable exit-code
  mapping as in-stream `task.failed` events. Previously a backend
  envelope code of `concurrent_work_limit` / `rate_limited` /
  `internal_error` returned by InitTask exited **1** (cobra default),
  while the same code surfaced mid-stream exited **4** — same
  condition, different exit code is precisely the agent-confusing
  inconsistency the `retryable` flag exists to prevent. After this
  patch both paths exit 4 and emit identical `task.failed` NDJSON
  events when `--output ndjson` is set. Verified end-to-end against
  the backend: a real `concurrent_work_limit` at InitTask now
  produces exit 4 + a structured terminal event with
  `retryable: true`.
- `vk create --output ndjson` now synthesizes a terminal
  `task.failed` event on stdout for **pre-stream** failures (InitTask
  errors). Previously a pre-stream failure left stdout empty,
  forcing NDJSON consumers to special-case "no terminal event implies
  it failed before the stream started". Every CLI exit ≠ 0 in NDJSON
  mode now ships exactly one terminal `task.failed` line on stdout.

### Docs

- `skills/vibeknow-create/references/events.md` rewritten against
  the actual emitted event taxonomy. Previously documented but
  never-emitted events (`task.submitted`, `task.queued`, `stage.*`,
  `task.cancelled`) removed. Engine-difference section added to make
  the pipeline-vs-agent split explicit. Schema version stays `"1"`;
  a `"2"` bump is reserved for renaming `node.*` → `stage.*` or
  `code` → `error_code`, neither of which ships in this release.

### Internal

- New `figlens.StreamEvent.NDJSONFields()` helper produces the wire
  shape both `vk create` and `vk video wait` emit. The two commands
  previously diverged in subtle ways (wait emitted `stage` / `node`
  on every event regardless of type and never emitted `code`,
  `retryable`, `video_url`); they now share one canonical projection.
- New `httpclient.IsRetryableCode(code)` centralizes the retryable
  inference so the SSE path no longer needs an ad-hoc switch.

## 0.6.2 — 2026-05-14

### Fixed

- `vk create --from <vectoria-uuid>` now recognizes the UUID form
  vectoria actually returns (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`,
  lowercase hex) as a doc-id and skips the upload step. Previously
  the `docIDRe` only accepted a hypothetical `doc_<alnum>` form that
  vectoria has never used, so re-passing the `doc_id:` line printed
  by a prior `vk create` run got treated as a local file path and
  failed with "stat: no such file or directory". Backward compatible:
  the legacy `doc_<alnum>` form still matches.

## 0.6.1 — 2026-05-14

### Changed

- Integration tests now build the `vibeknow` binary **once per `go
  test ./tests/integration/...` run** (via `sync.Once` + `TestMain`)
  instead of rebuilding inside each of the 15+ test functions that
  called `build(t)`. Removes redundant compile work on cold-cache CI.
- `runVideoCmd` helper signature changed from `(stdout, combined,
  exitCode)` to `(stdout, stderr, exitCode)`. Callers that want a
  combined view compute `stdout + stderr` explicitly. All hand-rolled
  `exec.Command(bin, ...)` blocks in the integration tests
  (`create_credits`, `create_engine`, `create_mode`, `kb_prune`)
  migrated to use this helper, removing ~80 lines of duplicated
  env-setup / ExitError-unwrap boilerplate.

### Fixed (housekeeping)

- `.gitignore` now covers `*.pdf` and `*.docx` (with `!test.pdf`
  preserved) so local smoke-test files don't appear as untracked
  candidates for accidental commit.

## 0.6.0 — 2026-05-14

### New

- `vk kb list` — list your vectoria knowledgebases with `--page`,
  `--size`, `--pattern <glob>`, `--older-than <duration>` filters.
  Glob uses Go `filepath.Match` syntax. Duration accepts `Nd` / `Nh`
  / `Nm` forms (`7d`, `24h`, `1h30m`).
- `vk kb delete <id>` — single-kb delete with confirmation prompt
  (skip via `--yes` / `VIBEKNOW_ASSUME_YES=1`). 404 from backend is
  treated as success (idempotent, `rm -f` semantics).
- `vk kb prune` — bulk-delete by filter. **Dry-run by default**:
  prints matched kbs without deleting; requires `--yes` to actually
  delete. Refuses to run without `--pattern` or `--older-than` —
  no "delete everything" shortcut. Partial-failure semantics:
  exit 7 if some succeed and some fail (matches `vk create --export`).
- `vectoria.Client.ListKBs(ctx, offset, limit)` — exposed for
  callers of the client library.
- `internal/durfmt.ParseAge` — duration parser with `Nd` day-suffix
  shortcut used by the kb filters.

### Migration

- New commands, no breaking changes.
- For users carrying the 0.5.x backlog of CLI-named orphan kbs:
  `vk kb prune --pattern 'vibeknow-cli-*' --yes` cleans them up.

## 0.5.2 — 2026-05-14

### Fixed

- `vk create` now cleans up the temporary vectoria knowledgebase it
  created when the pipeline fails before the backend task takes
  ownership (`InitTask` rejection: `insufficient_credits`,
  `script_invalid`, network errors, or any earlier upload/poll
  failure). Previously every failed `vk create` invocation left an
  orphan kb in vectoria forever — testing during 0.5.0 validation
  alone accumulated 6+ such orphans, and the user's tenant had
  accumulated 424 kbs total. The cleanup is best-effort: errors are
  swallowed so they don't mask the real failure the user is about to
  see. Once `InitTask` succeeds, the backend task owns the kb's
  lifecycle and the CLI no longer interferes.

### New

- `vectoria.Client.DeleteKB(ctx, kbID)` — exposes the existing
  vectoria `DELETE /v1/knowledgebases/{id}` endpoint, used by the
  cleanup above. Available to external callers of the client.

## 0.5.1 — 2026-05-14

### Fixed

- `vk create` now exits **5** (business failure) when the backend rejects
  `POST /v1/tasks/init` with `insufficient_credits` (envelope code 100001),
  matching the stream-side path's existing behavior. Previously this case
  exited 1 (cobra's generic error code), inconsistent with the documented
  exit-code contract and with the same condition surfacing later in the
  pipeline. Caught while running real-backend smoke tests during 0.5.0
  validation.

## 0.5.0 — 2026-05-14

### New

- `vk create --engine pipeline|agent` selects which figlens engine to
  invoke. Default `pipeline` keeps 0.4.2 behavior bit-identical;
  `--engine agent` routes to the v=2 agent engine
  (`/agent2forVideo/stream`, mirrors the web frontend's engine toggle).
- The v=2 agent engine emits free-form progress events without a node
  graph. The CLI now surfaces these as `node.progress` events in
  NDJSON output and `[agent] <message>` lines in text output, instead
  of silently filtering them out.
- `vk video status` / `vk create` JSON snapshot now includes an
  `engine` field (`"pipeline"` or `"agent"`) so agents can confirm
  which engine actually ran. The DB enum `"suite"` is remapped to
  `"pipeline"` for output to match the `--engine` flag's vocabulary.

### Changed

- `--engine agent --mode replica` is rejected at the CLI boundary with
  exit 2 and a clear message: replica is a v=3-only pipeline feature
  with no agent-engine analog (verified against go-figlens source).

## 0.4.2 — 2026-05-14

### New

- `vk create --mode replica` runs the figlens PPT/PDF page-by-page
  replica pipeline. `vk create --mode script` runs the verbatim-script
  ("讲稿锁定") pipeline that uses the uploaded document as the
  narration. Both modes are now visible to humans and AI agents
  through the same single-flag surface; default invocation (no
  `--mode`) is unchanged.
- `vk create --aspect horizontal|vertical` selects 16:9 or 9:16
  output. Accepts `16:9` / `9:16` as aliases.
- `vk create --bgm` enables background music (off by default).
- SSE progress events for the replica pipeline's new nodes
  (`doc_replica_plan`, `doc_replica_shoot`) now surface in `text` and
  `ndjson` output — previously filtered out by the CLI's stage map.

### Changed

- Script-mode preflight failures (`POST /v1/tasks/init` returns code
  `100004`) now exit **2** (validation, user fixes input) with the
  backend's localized message, instead of exit 5 with a generic
  "business error" label.
- `figlens.InitTask` now takes `InitTaskParams{KnowledgeID, DocID,
  VideoKind}`. Default-zero params produce the same wire body as
  before (`{"v": 3}`), so callers that don't use script mode are
  unaffected.

## 0.4.1 — 2026-04-24

### Fixed

- `vk auth status` now shows the refresh-token lifetime (the real session
  window) instead of the 2h access-token lifetime, so the countdown
  reflects when the user actually has to sign in again. Added a "days"
  duration unit so week- or month-long sessions render as "6 days"
  rather than "168h".
- `httpclient.parseBackendError` now prefers the envelope business code
  on HTTP 4xx/5xx responses. Previously account-service codes 110004 /
  110008 / 110013 on HTTP 401 were collapsed to generic `auth_required`;
  they now surface as `account_disabled`, `session_replaced`, and
  `account_pending_deletion`.
- On a permanently-dead session (replaced on another device, account
  disabled, account pending deletion), `OAuthTokenProvider` now purges
  the stored credential and returns a single `session_expired` error
  with the underlying cause preserved in `details.cause_code` /
  `details.cause_message` / `trace_id`. Previously the stale token
  lingered in the keychain and each subsequent command produced a
  confusing `business_error` message.

## 0.4.0 — 2026-04-22

### Breaking

- `vk video download` no longer auto-triggers export. If the MP4 is not
  yet rendered, it now exits 2 with a hint pointing to `vk video export`.
  Migrate: replace `vk video download <id> --session-id <sess>` with
  `vk video export <id> --session-id <sess> && vk video download <id> --session-id <sess>`.
- Text-mode `duration=` changes from raw milliseconds to human-readable
  (`duration=42s`). JSON `duration_ms` remains ms. Scripts parsing
  `duration=` as an int must switch to `--output json`.

### New

- `vk video export` renders the MP4 as an explicit step. Supports
  `--async`, `--yes`, `--timeout`, `--poll-interval`.
- `vk video export-status <export_task_id>` polls a specific export.
- `vk create --export` chains preview + export in one call. Exits
  **7** on partial success (preview ready, export failed).
- `vk video status` now returns a complete snapshot: preview state,
  export state, and `next_actions` hints.
- `--output ndjson` on `create` / `wait` / `export` streams one event
  per line for agent consumers.
- `VIBEKNOW_ASSUME_YES=1` / `--yes` skip confirmation prompts on paid
  operations.

### Fixed

- `vk create` no longer prints empty `video_path=` / `video_url=` lines
  at preview time. The pipeline's `share_url` (HTML preview page) is
  now the primary output.

## [0.3.3] — 2026-04-18
### Added
- All user-visible strings now route through the i18n table (`VIBEKNOW_LANG=en|zh`). Previously ~30 strings across `init`, `auth login/status/whoami`, `create`, `credits`, `video list`, and upgrade notices were hardcoded in Chinese and ignored the locale flag.
- `vibeknow update` actually checks the npm registry and reports `up to date` or the pending upgrade. Previously it was a P0 stub that only pointed at `npm update -g`.
- `VIBEKNOW_DEBUG=1` and `--verbose` now emit the HTTP request/response summaries the docs promised. The flag is sugar for the env var.
- `--output json` wired into `voice list`, `video status`, `video url`, and `doc get` — agents can parse the response directly instead of scraping text.
- `cmd/video/{status,download,url,wait}` and `doc get` route flag/argument validation through `clerr.Validation` so `--output json` gives a clean `{"ok": false, "error": {"type": "validation", ...}}` envelope with exit code 2.

### Changed
- User-facing flag help and package docstrings no longer reference internal Go-module names (`go-account`, `go-figlens`, `go-atlas`, …). External users see `Account service URL override`, `VibeKnow API service`, etc.
- `vibeknow doctor` assumes every backend exposes a `/healthz` returning `{"status":"healthy"}` on 200 or `{"status":"unhealthy"}` on 503 (matches `go-atlas` v0.3.6+). The transitional multi-path / multi-shape tolerance was dropped now that backends are aligned.

### Removed
- Deprecated `--api-endpoint` flag on `profile add` (old alias for `--endpoint-vibeknow`). Silent YAML migration via `config.Profile.APIEndpoint` remains, so existing `profiles.yaml` files still load.
- 5 redundant `staticToken` type declarations collapsed into one `httpclient.StaticToken`.
- Unused `clerr.{TypePermission, TypeNotFound, TypeRateLimit}`, `output.Select`, `output.Writer.Format()`, and `cmdutil.Factory.Endpoint()` (kept `TokenProvider`, which `Service` calls internally).

## [0.3.2] — 2026-04-18
### Fixed
- `vibeknow doctor` no longer reports spurious `[fail]` lines against services whose health endpoints live at `/healthz` or `/health` or return envelope-wrapped JSON. Probes `/healthz` → `/v1/health` → `/health` in order, accepts flat / envelope / atlas-style response shapes, and reports services with no exposed health endpoint as `[warn]` (not counted toward the failure exit code).

## [0.3.1] — 2026-04-18
### Fixed
- `CloudDefaults` pointed at unreachable placeholder hostnames. Fresh `npm install -g vibeknow-cli && vibeknow init` now reaches the device-code step instead of failing DNS resolution.

### Added
- `vibeknow auth status --output json` emits a machine-parseable envelope (`authenticated / profile / source / auth_method / token_status / expires_at / user`) for AI Agents and CI.
- `vibeknow init` mirrors the `VIBEKNOW_TOKEN` env-var warning already emitted by `auth login`, so users who carried over old credentials get an early signal before the wizard stores a keychain token that would be shadowed by the env var.
- Regression test pinning the `vibeknow auth login --no-wait` JSON envelope shape.
- Sanity test guarding `CloudDefaults` against typos (every URL parses as an absolute `https://` URL).

### Changed
- README (English and Chinese) Quickstart rewritten to match the shipped `vibeknow init` flow (humans) and the two-phase device flow (`--no-wait` / `--device-code`) for AI Agents. The "Coming in v1" Device-Flow note is removed — it has shipped.
- Hero Command example stops hardcoding a specific voice ID; users are directed to `vibeknow voice list` first.

## [0.2.0-p1] — 2026-04-15
### Added
- Multi-endpoint direct-connect: profile schema v2 with `endpoints` map for account/vectoria/figlens/vibeknow. Cloud defaults built in.
- `internal/httpclient` stack: core client + middleware chain (auth / trace-id / version skew / verbose+redact / retry).
- `internal/errs` canonical Error Object (spec §11.2).
- `client/account` with `Whoami`.
- `cmd/auth whoami / status / logout` (no interactive login in P1; use `VIBEKNOW_TOKEN` env or P1.5's Device Flow).
- `cmd/api call --service <name> --method ... --path ...` raw tunneling.
- `cmd/doctor` extended with concurrent endpoint reachability + API version probe.
- Backend contract document at `docs/contracts/p1-backend.md`.

### Changed
- `profile add` accepts `--endpoint-{account,vectoria,figlens,vibeknow}`; `--api-endpoint` retained as deprecated alias for `--endpoint-vibeknow`.
- `profile show` prints endpoints map instead of single `api_endpoint`.
- Profile schema_version bumped from "1" to "2"; v1 profiles auto-migrate on load.
- `doctor` header message updated to reflect P1 scope (environment + endpoint diagnostics).

### Deferred
- Interactive `auth login` (Device Flow + PAT) → P1.5 standalone project.
- Service clients for vectoria / figlens / vibeknow / speech → P2 (alongside shortcuts).

## [0.1.0-p0] — 2026-04-15
### Added
- Repository scaffold with cobra-based `vibeknow` CLI.
- Cross-platform build toolchain (`Makefile`, `build.sh`, GitHub Actions CI).
- Framework packages: `internal/charcheck`, `internal/redact`, `internal/i18n`, `internal/output`, `internal/lockfile`, `internal/keychain`, `internal/credential` (file store with AES-GCM + scrypt; env/keychain/file resolver), `internal/config` (profiles schema with validation, profiles.yaml r/w, generic KV).
- Base commands: `vibeknow version`, `vibeknow --version`, `vibeknow completion {bash|zsh|fish|powershell}`, `vibeknow update` (P0 stub), `vibeknow doctor` (local checks), `vibeknow profile add/list/use/remove/show`, `vibeknow config get/set/list`.
- Global flags: `--profile`, `--output`, `--verbose`.
- npm distribution via `@vibeknow/cli` with per-platform postinstall binary selection.
- End-to-end integration smoke test.

### Known limitations (P0)
- No `auth login/logout/whoami` (needs go-account client — P1).
- No backend-calling commands (P1 / P2).
- No `create` / `video` / `doc` / `rag` shortcuts (P2).
- No Skills published (P3).
- `update` command is a stub.
