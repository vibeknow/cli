# Changelog

## Unreleased

### Changed — where the binary is fetched from, and how long that is allowed to take

`scripts/install.js` tried GitHub Releases first and
`registry.npmmirror.com/-/binary/vibeknow-cli/` second. The second does not
exist — the package is not registered for binary sync there — so in practice
there was one source, and it is the one most likely to be unreachable from a
corporate or mainland network. Each attempt was allowed 120 seconds, and a
WorkBuddy connector's whole `npm install -g` gets 300, so a host that was
reachable-but-stalled could spend the entire budget before the fallback was
even tried.

The source list now starts with `VIBEKNOW_BINARY_BASE_URL` when it is set,
which is what lets a mirror change without publishing a package: a connector
passes it through its `cli.json` `env` field, and npm hands the install
command's environment to postinstall. Per-source wall clock drops to 45
seconds, and a failed attempt now prints why — with several sources tried in
turn, that line is the only thing separating "this host is blocked here" from
"this version was never published".

Releases also learn about pre-releases. `npm publish` puts every version on
`latest` unless told otherwise, so tagging `v0.8.0-rc.1` to rehearse the
install path would have handed that build to every existing user. A tag
carrying a pre-release part now publishes to the `rc` dist-tag and marks the
GitHub release as a pre-release, and the skill-version gate compares against
the release being rehearsed rather than the rc string.

### Fixed — the same progress line appeared twice in one payload

`video wait --for` emitted both `stage` and `message` even when they were the
same string, which they always are on `--engine agent`: that line reports no
node names at all, so the free-form line *is* the stage. One fact written
twice reads as two, and a caller comparing them looks for a difference that
is not there. `message` is now emitted only where it adds something a node
position does not.

### Fixed — a finished pipeline node was reported as the current one

`video wait --for` recorded the last node it heard about but not whether that
node had ended, so a run reported the same position whether it was inside a
node or long past it. On the hand-drawn line that is most of the run: it
finishes `script_writing`, then draws in silence for minutes, and every check
during that stretch answered "outline / script_writing" — naming work that
had completed, on the one line where a person is most likely to conclude
something has stalled and start over at full price.

A finished node now reads `past <node>`, and on the hand-drawn line
`past <node>; drawing (this line reports no progress until it finishes)`.
Only the absence is explained; nothing here claims the run is healthy, which
is not knowable from silence. A `node.warning` neither moves the run nor
closes the node, so it no longer affects the answer either way.

Found by running each creation mode against a real account, which is also
what established what that line actually emits: `handdraw_*` nodes emit
nothing, but the shared `script_writing` / `tts_generate` / `bgm` /
`video_package` nodes do, so the silence is a middle rather than the whole.

### Fixed — `video script` reported "0 shots" as a success to every agent caller

The empty-shot-list check sat after the JSON branch had already returned, so
one work gave two different answers: exit 6 with an explanation to a person,
and exit **0** with `scene_count: 0` and an empty `script` to a script. A
script is what every agent caller uses — the skill tells it to always pass
`--output json` — so the path with the explanation was the one nobody took.
What came out the other end was "this video has 0 shots", stated as a fact
about a video that had simply not recorded any.

The check now runs before the format branch, and separates two causes that
need opposite responses. A run still generating is exit 6, which means come
back. A work that will *never* have shots is exit 5, which means stop asking:
the agent engine (一键成片) renders without a storyboard, and a failed or
deleted run never got that far. Exit 6 on those was an instruction to wait
for something that is not coming.

Found by running the connector against a real account, where all ten existing
works happened to be agent-engine ones — the case the tests did not have.

### Added — `video wait --for`, waiting in steps a chat client can take

A render takes minutes. A caller whose own tool gives it two — which is what
an agent in a chat client has — could not use `video wait` at all: it blocks
until the run finishes, so the call was killed mid-stream and the run read as
a failure it was not.

Polling `video status` instead was the obvious escape and it does not work.
The snapshot's preview half is one boolean, `ready`, because it is built from
the work row and the work row learns nothing until the export stage. So a
caller checking every ten seconds got `ready: false` ten seconds ago, `ready:
false` now, and had nothing to tell the user either time. The progress does
exist — the event stream carries the stage, and `internal/stage` already
translates it — it was simply not reachable from a command that returns.

`--for 90s` keeps the stream and gives it a deadline: watch for that long,
then report the stage reached and stop. `next_actions` hands back the same
command to run again. Nothing is lost between calls and nothing is billed
twice; the run belongs to the backend, not to the process watching it.

It exits **6**, not 0. Exiting 0 would mean the task succeeded, and a caller
running `wait && download` would be sent after a video that is still being
made. Exit 6 already means "not terminal, reattach rather than start over",
which is the right next move — and `reason: "wait_budget_expired"` separates
it from the two other things 6 covers, a broken stream and a paused task,
both of which are states nobody asked for. A budget of zero or less is
refused rather than treated as "no budget": silently becoming an unbounded
wait is the exact hang the flag exists to prevent.

The reported stage is the last position on the wire, so it advances between
calls. Two cases report `no stage reported yet` and neither is a fault: the
first seconds of any run, and the hand-drawn line, whose whole middle section
emits nothing.

### Added — `create --text` and `--from -`, for text that was pasted rather than saved

`--from` took a path, a URL or a doc_id, so a passage the user pasted into a
conversation had no way in. The workaround was to write a temp file through
the shell, which mangles multi-line text on quoting and then names the
document after whatever the temp file was called.

`--text` takes the text itself; `--from -` reads it from stdin, which is what
long or multi-line content should use — a quoted heredoc puts the text through
untouched. Either way it is uploaded as a document named after its opening
line, and everything downstream behaves exactly as it does for a file,
`--script-lock` included: "照着我这段话念" is now a command.

Empty or whitespace-only text is refused, and so is anything over 512 KB —
the pipeline truncates document content to 8000 runes before any node sees it,
so a larger upload buys nothing and is more likely a mistake. Passing both
`--text` and `--from` is refused too: they name the same thing, and picking
one would leave the caller with a video built from an input it did not choose
and no way to find out. `--text` is not allowed in a `--preset`, for the same
reason `--from` is not: it identifies one run's input, not a reusable style.

**This is not generation from a topic.** The content is the user's; nothing
here invents any. A request with no material at all still has no command —
both engines fetch the document before anything else runs (go-figlens
`internal/pipeline/node/video_knowledge.go`, `internal/service/trpc_assistant.go`)
and fail without one.

### Added — `video status --preview-dir`

`--preview-dir` was available on the commands that run a job, which meant
only the process that started a run could ever put its cover or its MP4 on
disk. An agent that reattached to a task — someone else's, or its own after
its context was discarded — could name the artifacts and not hand them over.

`status` now takes the flag too, delivering whatever the work row currently
points at: the cover from the moment the preview exists, the MP4 from the
moment an export has produced one. Both are skipped when absent, so a
snapshot taken mid-render still costs nothing and delivers nothing — no
signing round-trips either. A `--preview-dir` that cannot be written is
refused before the request, as it is on `create`.

### Added — `subtitle fonts` / `subtitle presets`, and the rest of the subtitle style

`--subtitle-font` has always been documented as "a family the backend
allows", with no way to find out which those are. A wrong guess came back as
`fontFamily not allowed`, naming neither the alternatives nor where to look
for them — the worst shape an argument can have for a caller that cannot see
the catalog. `vibeknow subtitle fonts` lists it. Both the `#` and the exact
family work as `--subtitle-font`.

`vibeknow subtitle presets` lists the eleven ready-made looks the product
ships, and `video set --subtitle-preset` applies one. This matters more than
it sounds: subtitle readability is a combination rather than a set of
independent settings. The outlined looks also clear the background plate; the
plated looks also switch the outline off. Assembling one by hand from
individual flags is easy to get half right, and half right exits 0 — nothing
on the wire is wrong, the video just looks bad. A preset carries both halves.

A preset patches only the fields that make up its look, so size, vertical
position and entry animation survive it, and any `--subtitle-*` flag passed
alongside applies on top: `--subtitle-preset 2 --subtitle-size 52` means
"that look, but bigger".

Six style fields that the client already modelled but no flag could reach are
now reachable: `--subtitle-font-weight`, `--subtitle-bg-color`,
`--subtitle-bottom`, `--subtitle-stroke-color`, `--subtitle-stroke-width` and
`--subtitle-animation`.

Their ranges are checked before anything is sent, which is not symmetry for
its own sake. The backend *clamps* position and outline width instead of
refusing them, so asking for a subtitle at 1.5× the frame height succeeded
while storing 0.98, and nothing reported the substitution — the command said
it worked and only the video disagreed. Font weight it does not validate at
all. Refusing locally costs one exit code and states the range.

`video set`'s text output no longer prints the resulting style through Go's
default struct formatting. `{Source Han Sans 36 0 #ffffff rgba(8,8,12,0.68) 0
#000000 0 fade}` names no field and pads the gaps with zeroes that were never
set; it was survivable when a change touched one field and is not now that a
preset sets six. Fields are listed one per line, and only the ones actually
being stored. The report is also sorted, so repeating a command does not
reorder its own output.

### Added — `video edit`, the first change to what a video actually says

Until now the CLI could read a video's narration but not change a word of
it. Fixing one wrong sentence meant a fresh `create` at full price, throwing
away everything about the video that was already right.

`vk video edit --scene N --script "…"` replaces one shot's narration and
regenerates that shot. `--scene` uses the numbers `vk video script` prints.
`--script-only` regenerates the voice-over alone, leaving layout and
background image untouched; the default rebuilds those too, which is what a
rewrite of a noticeably different length needs, since nothing re-flows
without it.

This bills, so it goes through the same gate as `video export` under a
second boundary, `scene_edit_confirmation`. Its payload carries `from` as
well as `to`: the user is approving a diff, and showing only the replacement
asks them to agree to a change they cannot see. Both halves are hashed into
the `action_id`, which gives two properties for free — an agent iterating on
wording cannot carry one approval across several rewrites, and a shot that
changed underneath between the block and the resume invalidates the token
rather than being silently overwritten. `script_only` is hashed too, so
approving the cheap regeneration never authorises the expensive one.

Unlike an export, no credit count is quoted. What an edit costs depends on
how much text the model writes and how long the resulting speech runs, and
the backend does not say in advance; the prompt names the kinds of billed
work instead. A number invented to fill the field would undo the only thing
the gate rests on.

Four mistakes are refused locally, before anything is sent or billed, and
they stay refused under `--yes`: a `--scene` outside the shot range (the
message names the range), narration identical to what the shot already says,
empty narration, and a missing `--scene` or `--script`.

Two things a caller has to be told and would not otherwise learn. The
rendered MP4 is **not** withdrawn — the backend leaves it in place, so
`video download` keeps returning a video of the previous narration until the
next export; the response reports this as `export_stale` with a
`next_actions` entry. And there is **no undo**: the backend keeps a rollback
snapshot to recover from its own mid-run failures but exposes no way to ask
for a previous version back.

The edit lock and the credit precheck are both answered before the event
stream starts, as a JSON envelope on HTTP 200. Read as a stream they would
look like a connection that carried no events — a generic exit 1. Read as an
envelope they are `work_edit_busy` (exit 4, transient, retry) and
`insufficient_credits` (exit 5).

### Fixed — a terminal SSE event larger than 64KB ended the stream

`bufio.Scanner`'s default line limit sat under the size of a completion
event carrying a full rendered package. Past it the scan stopped, and the
run read as "the backend went quiet" on the one event that says the work is
finished. The limit is now 8MB.

### Added — `create --preset`, a reusable style in a file

`create` now takes 21 flags, and most of them describe how a video should
look rather than which run this is. A team that has settled on its house
style re-typed a dozen options every time, and an agent driving the CLI had
to carry that combination in a prompt, where it drifts. `--preset` puts it
in a YAML file: a name resolved under `<config>/presets/`, or a path.

A preset only supplies defaults. Anything also given on the command line
wins — including an explicit `--bgm=false` — so a file can never contradict
what the caller typed.

The option set is an allowlist, not "every create flag". A preset may not
set `--export`, `--yes` or `--confirm`: those authorize a charge, and
consent that arrives inside a file someone forwarded to you is not consent.
It may not set `--from`/`--kb-id`, which identify one run's input, nor
`--async`/`--preview-dir`/`--output`, which describe one invocation. Each of
those is an explicit exit 2 naming the key and the reason rather than a
silent drop — a `yes: true` that were quietly ignored would read as though
it had worked.

Everything is rejected before the first upload, so a broken preset costs
nothing: an unknown key (with the valid list), an unsupported
`schema_version`, an empty `create:` block, or a value the flag itself
refuses. Every run reports what it applied — a `preset.applied` event with
the sorted list of keys that took effect, or one stderr line in text mode —
because with a preset in play the command line no longer answers "what was
this run configured with".

Client-side expansion only, with no backend involvement: a preset can
express exactly what the command line can express and nothing more.

### Added — `video set`, the first changes that do not mean regenerating

Music, subtitles and the title can now be changed on a finished video. These
are the settings the pipeline does not have to run again for, so they cost
nothing and take effect at once — which is what makes them worth having in a
tool driven by conversation, where "turn the music off" is a sentence and a
full re-run is a bill.

Subtitle style is read, modified and written back rather than written. The
endpoint stores what it receives with no merge, so sending only the size the
user asked about would have cleared the font, weight, colour, outline and
animation with it — "make the subtitles bigger" would have quietly reset
everything else about them, reporting success. A test pins the merge by
asserting the untouched fields survive.

Every one of these changes except the rename discards the work's rendered
MP4 server-side, because the change has to be baked into the file. The
preview and share link keep working; the download does not, until the next
export — which bills. That is reported as `export_invalidated`, with the
export command in `next_actions`, and stated on stderr in text mode: a user
who turns the music off and later finds no file has no way to connect the two
on their own.

Presentation only. Narration, images and layout still have no edit path and
still mean a fresh `create` at full price.

### Added — `video script`, which reads what a video actually says

Everything a caller could learn about a finished work described the
container: its title, duration, cover, share link. The narration was not
readable at all. It exists in the generation stream only as `script_chars` —
a character *count* — so once a run ended, "what does this video say" had no
answer short of watching it, and "show me the script" had no command behind
it.

`video script` returns the shot list with its narration: per shot the text,
duration, layout and status, plus public URLs for the still, the narration
audio and the subtitles. It also returns the whole script stitched in order,
because that is the form the request usually takes and every caller would
otherwise rebuild it. Absent media is omitted rather than emitted empty — a
missing key reads as "not rendered yet", an empty string reads as a broken
link.

Read-only and free: it queries stored rows, re-runs nothing and bills
nothing, so a conversation can come back to it as often as it likes. An
empty shot list exits 6 rather than 0, since a run that has not produced
shots yet is a *not yet*, not a silent video.

This closes the reading half of the post-generation gap. The editing half is
still open: nothing here changes a script, an image or a subtitle, and
adjusting content still means a fresh `create` at full price.

### Added — `video pause` and `video resume`

A run could be started and then only waited out. That is a poor fit for a
CLI driven by an agent acting on someone's sentence: the wrong document, the
wrong aspect ratio, or a change of mind arrives *after* the run is going, and
until now the only response was to let it finish and pay for a video nobody
wanted. Pause stops it and keeps the work already done; resume continues from
there rather than starting over.

Resume also covers the failed case, which matters more than it sounds. It
restarts from the last checkpoint and the backend reopens the original bill,
so it costs a fraction of the alternative an agent would otherwise reach for
— creating the video again, at full price, discarding every scene that had
already succeeded. The returned `mode` says which of the two happened, since
the caller has to be able to tell the user what it just did.

The backend refuses several ways, all as HTTP 400 with a sentence, which lands
on exit 2 — "your input is wrong, fix it and retry". For these that is wrong
in the expensive direction: "this engine keeps no checkpoint", "content policy
stopped it" and "the run is not in a pausable state" are facts about the run,
not the arguments, and an agent told to fix its input will keep permuting
flags against a condition no flag can reach. They now exit 5. The single
genuinely retryable refusal — a concurrent pause/resume on the same session —
is carved out to exit 4.

### Changed — `auth status` exits 3 when there is no usable credential

It used to exit 0 whichever answer it gave, on the reasoning that reporting
successfully is not a failure. That reads fine for a person and badly for the
program this command mostly talks to. A connector host decides whether to
start a login from the exit code — WorkBuddy's connect sequence is *run
status → exit code ≠ 0 → run auth* — so exiting 0 while unauthenticated
invites it to skip the login and show a connected card for a machine holding
no credential, where every command then fails with this same code 3 anyway.

The spec is not self-consistent about this: its detailed decision table
consults `statusMatchJson` after the exit code, and by that reading exiting 0
was survivable. Being correct under only one of two readings bought nothing,
so the exit code now agrees with the payload under both.

Only the exit code changed. stdout still carries the same JSON, and nothing
is written to stderr — the report is the message, and a caller parses it. A
pending authorization counts as not connected, which is what keeps a host
polling rather than concluding it is done.

### Fixed — a refresh that could not be saved was replayed against the server

When the keychain cannot be written, a refresh has already succeeded on the
wire: the server has spent the presented token and handed back a new pair
that now cannot be stored. The code kept that pair in memory and deleted the
superseded entry — but a keychain that refuses writes refuses deletes for the
same reason, so the stale copy survived. Reads preferred the keychain
whenever it merely *read*, so everything after that in the process got the
token the server had just spent.

Presenting it again is not a retry. A rotating server reads a second
presentation as a stolen credential, revokes the whole session, and logs a
security warning; the user is signed out minutes later with nothing to
connect it to. Expiry now decides which copy wins, which stays right in both
directions — the unwritten copy beats the stale one it superseded, and a
token another process has since written beats ours.

Latent while go-account re-issued without spending the presented token. It
became live when refresh moved onto `jwt.Refresh`.

### Fixed — waiting for the refresh lock ignored the caller's deadline

The cross-process lock was acquired under a private thirty-second budget
built from `context.Background()`, which silently outranked every deadline a
caller had set. `auth status` allows itself five seconds because a connector
host polls it every three and abandons it at ten; on a slow network, where
every poll refreshes and the waiters queue up, it could sit on that lock well
past the point the host had already recorded a disconnection. The wait is now
bounded by the caller's context, with the thirty seconds kept only as a
backstop for callers that set no deadline.

### Fixed — `auth logout` payload was missing `revoked` in one branch

Logging out with no profile at all emitted an envelope without the `revoked`
key the other paths carry, so a caller had to know which case it was in
before reading the result. All three outcomes now report the same shape.

### Fixed — `auth status` reported a live connection for a credential the server rejects

`authenticated` was decided from the stored token's own expiry timestamps.
That answers "has this credential run out of time", which is not the same
question. A credential also dies when the server stops accepting it — the
signing key was rotated, the session was revoked, the account was disabled —
and none of those leave a mark on the copy in the keychain. It goes on
looking valid for the rest of its nominal lifetime, so `status` reported a
healthy connection while every actual command failed with exit 3.

This is not hypothetical: go-account rotated its production signing secret,
which invalidates every credential issued before that deploy. A connector
host drives reconnection off `status` alone and polls it every three
seconds, so it would have shown a connected card, indefinitely, with nothing
prompting the user to reconnect.

`status` now asks the server whether the credential is still accepted, which
is what the connector spec recommends in the first place (§12, 方案 B). Three
outcomes, deliberately kept distinct:

- accepted → `authenticated: true`, with the server-confirmed identity
- **rejected** → `authenticated: false`, and the dead credential is purged
- **no answer** (timeout, 5xx, unreachable) → the local verdict stands

Collapsing the last two would have been the easy mistake and a worse bug
than the one being fixed: a few seconds of bad network would tear down a
session that was never in any trouble.

The check runs through the profile's real token provider rather than a
static token, so a stale access token is refreshed here exactly as any other
command would refresh it — otherwise the most common case of all, a
connector left idle overnight, would report its perfectly good session as
dead. The budget went from 3s to 5s to cover that extra round-trip, still
well inside the 10s a connector host allows.

### Fixed — a missing login was reported as a retryable network error

Every command run without a usable credential exited **1** with
`[network_error] … no credential found`, and the error claimed to be
retryable. Both halves were wrong, and both mattered to an agent driving
the CLI: exit 1 is the catch-all, so "the user has to connect an account"
was indistinguishable from "the API misbehaved", and `retryable: true`
invited a retry loop against a condition no retry can clear.

The cause was in the shared HTTP layer. `AuthMiddleware` aborts the
request with a proper auth error, but `http.Client` wraps every transport
failure in `*url.Error`, and the layer above looked only for its own
error type before rebuilding everything else as `network_error`. It now
preserves a classification that was already made. Affected every command,
not just recent ones; `create` additionally no longer degrades past an
auth failure during prompt optimisation, which used to print a confusing
"using default" line before dying at init anyway.

### Fixed — a missing expiry field logged the user out and deleted the credential

Token expiries were computed as `now + expires_in` unconditionally. An absent
field decodes to zero, so the arithmetic produced `now - 30s` — a deadline
already in the past. A credential minted from a response that omitted
`refresh_expires_in` was therefore *born expired*: the next command read it as
dead and purged it from the keychain. One missing field in a backend response
was enough to log a user out immediately after a successful login, with no
error anywhere to explain it.

Zero now means "not stated" and is stored as an unset expiry, which `Status()`
already reads as "no information". A negative lifetime is still honoured — a
deadline in the past is information, not the absence of it.

Relatedly, a refresh response that returns only a new access token — which
RFC 6749 §6 explicitly permits, and is what any server that does not rotate
refresh tokens sends — was stored verbatim, blanking the refresh token and
back-dating its expiry. That destroyed a working session on its first refresh
and forced a new login every access-token lifetime. What a refresh response
omits now leaves the stored credential unchanged.

### Fixed — a killed session was reported as a network error

The 401 retry path had the same flaw as the missing-credential path: when a
forced refresh found the session permanently gone — replaced on another
device, account disabled — that error was returned through `RoundTrip`,
wrapped by `http.Client` in `*url.Error`, and rebuilt as a retryable
`network_error`. The user was told their connection was flaky and to try
again, when their session had been killed and only logging in would help. The
structured code now survives the transport layer, so it exits 3.

### Changed — the 401 retry no longer buffers the request body

To make a request replayable the retry middleware read it fully into memory
first. Free for JSON, but document upload streams the file through an
`io.Pipe`, so the buffering pulled entire decks into RAM and cancelled out the
streaming it was built on — to cover a 401 that is already unlikely, because
a near-expiry token is refreshed *before* the request goes out. Replay now
uses `net/http`'s own `GetBody`, which exists for every in-memory body; a body
that cannot be regenerated is sent once and its 401 surfaced as the auth error
it is.

### Fixed — a device login no longer dies with the process that started it

`auth login --headless` polls until the user authorizes, but the host that
spawns it decides how long it lives — the WorkBuddy connector spec caps
the auth subprocess at five minutes while the device code is valid for
fifteen. The code existed only in that process's memory, so a user who
took their time authorized successfully in the browser and the CLI never
found out: the connector sat at "not connected" with no way forward but
authorizing all over again.

The pending authorization is now parked in the config dir (0600) before
the URL is printed, and **`auth status` completes the exchange** — which is
the flow the connector spec recommends for device-code CLIs, and needs no
host cooperation, because connect and reconnect both poll `status`
already. Also in this area:

- `auth status --output json` reports `pending_authorization: true` while
  a login is open and waiting on the user — distinct from being logged
  out, and the difference between "wait and re-check" and "start a login".
- `auth logout` clears the parked code (otherwise the next `status` would
  silently sign the user back in) and now succeeds when there is no
  profile at all, instead of erroring on a disconnect that has nothing to
  disconnect.
- `auth status` bounds its identity lookup at 3s. It previously inherited
  the shared 30s HTTP timeout, well past the 10s a connector host allows
  for a status check — a stalled network read as "disconnected" and
  flapped a connection that was fine.
- `auth login --headless` no longer prints the raw `device_code`. It is a
  live credential and the host captures stdout into its logs; nothing
  outside the process needs it. `--no-wait` still prints it, since there
  the caller must resume with `--device-code`.
- Failed logins (code expired, authorization denied) exit **3** instead of
  1.

### Fixed — quota exhaustion exited 1

`project_quota_exceeded`, `project_works_full` and
`tts_preview_quota_exceeded` fell through to the generic exit 1, reading
as "something went wrong" when nothing about the request was wrong. They
now exit **5**, matching `insufficient_credits`: the run failed for a
reason the CLI cannot fix, and the answer is to tell the user what ran
out rather than to retry.

### Fixed — `voice list --output json` hid cloned voices from `templates`

The flat `templates` array listed public presets only, while the human
table and `--voice` resolution both included the caller's cloned voices —
so the same command answered "what can I use?" differently depending on
`--output`. `templates` now means every voice `--voice` accepts.

### Changed — stage map rebuilt against what the backend actually sends

The progress stage table predated two backend reworks and had drifted into
fiction: six of its node names no longer exist on the wire
(`text_speech`, `content_analyze`, `video_director`, `design`,
`scene_generate`, `theme_select`), one was a wrong guess that never
existed (`image2_style_select` — the real step_id is
`image2_theme_select`, so the image mode's style-selection stage never
lit), and `big_director` — the standard line's longest LLM node, one to
two minutes of planning — was missing entirely. Meanwhile the `parse` and
`suggest` stages could never light at all: the backend registers no
progress events for the nodes that fed them.

The table now mirrors the backend's event registry exactly. The stage
vocabulary is `outline → tts → render → publish` (four, not six); nodes
the backend adds later degrade to free-form progress lines instead of
being dropped. Consumers keying on `stage` values should drop `parse` and
`suggest`, which never actually arrived.

### Added — the five online creation modes, fully addressable

The CLI's mode surface now matches the product's online lineup end to end:
灵活创作 (default), 一键成片 (`--engine agent`), 图解视频 (`--mode
image`), PPT 讲解 (`--mode replica`), 手绘动画 (`--mode handdraw`), plus
the orthogonal 原稿锁定 switch (`--script-lock`). New on top:

- **`--theme`** picks the visual style; **`vk theme list --mode <mode>`**
  browses each mode's catalog (design / image / hand-drawn suites, the
  latter with preview URLs). Suite membership is validated by the backend;
  the agent engine rejects the flag locally rather than ignoring it.
- **`--language`** sets the output locale for script + narration
  (zh-CN, en-US, es-ES, fr-FR, pt-BR, ja-JP, ko-KR). Unknown values fail
  locally with exit 2 — the backend would silently fall back to the
  deployment default, discarding an explicit choice.
- **`--pages` now travels on init too**, so the image-mode feasibility
  preflight (word count ≥ pages × 50, mandatory images ≤ pages) runs
  against the real page count instead of the backend's default of 4.
  Range-checked locally at 1–20.

### Changed — `vk voice list` speaks the multi-language catalog

Voice listing moved from the flat `/voice-templates` endpoint to
`/pipeline-voices`, the one the product's voice picker feeds from: public
templates arrive grouped by language and the signed-in user's cloned
voices ride alongside (usable with any `--language`). `--voice <#>` now
resolves cloned voices too. `--language <locale>` filters the public
groups; JSON output keeps the flat `templates` array (now with a
`language` field) and adds `languages` + `cloned`.

### Added — silence is now survivable

Two kinds of quiet used to be indistinguishable from a hang:

- **`task.paused`**: a run paused from the web editor now surfaces as its
  own event instead of a dangling stream. `create` and `video wait` exit 6
  with the resume path spelled out, and the run ledger records the run as
  `paused` rather than `unknown`.
- **Stall notice**: the hand-drawn line's whole middle section (style,
  storyboard, drawing, vectorize) emits no progress events by design —
  minutes of nothing on a healthy run. On the human path the CLI now
  prints a "still generating" reassurance after 60 s of stream silence,
  with a hand-drawn-specific wording; structured outputs are unchanged
  (the contract there is: silence is normal, wait for a terminal event).

Also in the stream contract: the backend's enveloped `[DONE]` sentinel
terminates the read (EOF remains the fallback), data-frame keepalives are
ignored by type rather than by accident, and `node.succeeded` events now
carry the backend's `metrics` (real node outputs: `script_chars`,
`chapters`, `duration_sec`, …) through to NDJSON and `vk_event` consumers.

### Added — talking-head avatars

`vk create --avatar sys_<id>|ua_<id>` puts a presenter in the video, with
`--avatar-position` (four corners) and `--avatar-size` (120–480 px circle
diameter at 1080p base); omitted options fall back server-side to the
user's saved preference, then defaults. `vk avatar list` shows the public
presets — each with its paired `voice_id`, so face and voice stay matched
— alongside the user's own trained avatars and their states.

The avatar wire fields are camelCase (the backend binds them that way);
local validation mirrors the backend's hard 400s (reference shape,
position enum, size range) so a bad flag fails before anything is
uploaded or billed. Rejected up front with `--mode handdraw` and with
`--engine agent`: both would be accepted by the backend and then silently
rendered without the presenter (no hand-drawn avatar node; v2 stores the
config but has no compositing yet).

MP4 export of an avatar work is gated server-side until every scene's
presenter has rendered; failed scenes block it permanently. New
`vk video avatar-retry [task_id] [--scene N]` resets exactly the failed
scenes (script, images, TTS, healthy scenes untouched; nothing re-billed)
so the export can proceed.

### Added — preflight codes 100007–100011 mapped

`image_invalid` (100007, image-mode page-count preflight; exit 2),
`work_edit_busy` (100008, scene-edit lock; retryable, exit 4),
`project_quota_exceeded` (100009), `project_works_full` (100010),
`tts_preview_quota_exceeded` (100011). Previously all five collapsed into
`business_error`.

### Docs — `auth login --headless` was never recorded here

The flag shipped earlier (single command, no TTY: device-code envelope on
stdout, poll in place, write the keychain, exit 0 — the shape WorkBuddy's
connector runner drives), but the changelog never said so. Recorded now
for the release this entry ships in.

### Added — progress and result from one invocation

`--output json` was silent until the end; `--output ndjson` put the
progress stream on stdout, where it displaced the final answer and left a
consumer to work out which of N lines was terminal. Watching a run and
parsing its result was a choice nobody should have had to make.

Long-running commands now write progress to **stderr** as
`vk_event={...}` lines while stdout keeps carrying exactly one document.
The payload is the same shape as the NDJSON stream, so one parser serves
both. On by default with `--output json`; `VIBEKNOW_EVENTS=1` / `=0`
overrides in either direction. Text mode keeps its prose — stderr there is
a person's progress display — and ndjson keeps its stdout stream unchanged.

### Added — `--preview-dir`: artifacts an agent can actually hand over

`share_url` is a hosted page. A caller driving the CLI from a terminal
cannot open one, so the single output of a video tool worth looking at was
the one output it could not pass on.

`--preview-dir <dir>` on `create`, `video wait` and `video export` writes
the run's artifacts there and announces each as a `resource_ready` event
carrying an absolute `local_path` to a fully written file. Downloads land
in a temp file and are renamed last, so a reader never sees a partial one.

Deduplicated by content hash against what is already on disk, not by URL —
the backend re-signs unchanged assets, so keying on the address would
re-deliver the same still on every poll. Because the comparison is against
the file, it survives across processes. A failed fetch emits
`resource_preview_warning` and never fails the run.

The source URL is deliberately absent from every event: those are signed,
and an agent that relays one has published a credential.

### Changed — a paid render is no longer confirmed on the user's behalf

`vk video export` and `vk create --export` with no terminal attached used
to proceed, spending a credit, with a note on stderr nobody was reading. A
confirmation prompt assumes someone is there to answer it; when an agent
runs the CLI nobody is, and both available answers were bad.

The decision is now a value rather than an interaction. Without a TTY and
without prior authority the command exits **8** and writes to stdout:

```json
{ "status": "blocked",
  "pending_actions": [{ "action_id": "act_…", "blocking": true,
    "payload": { "credits": 1, … },
    "options": [{ "id": "confirm", "effect": "resume" }, { "id": "cancel", "effect": "none" }],
    "resume_command": "vk video export 42 --session-id s_x --confirm act_…" }] }
```

The caller relays the question and the token; `--confirm <action_id>`
proceeds. The token is an HMAC over the action and its decision-relevant
payload under a per-installation key, so it cannot be derived by reasoning
and stops verifying if the terms change — consent is to a specific price
for a specific run.

This is not a security boundary and does not pretend to be one; anything
running as the user can read the key. It is an evidence boundary against
confident invention.

**Breaking for unattended callers that relied on the old auto-confirm.**
`--yes` and `VIBEKNOW_ASSUME_YES=1` still bypass the gate and are the right
answer when the user authorised the spend in advance; every script in this
repo's own tests already used them.

Scoped to the two paid paths. `kb delete` keeps the plain prompt — it is
destructive but not billed — and `kb prune` is already dry-run by default.

### Changed — exit 6 says whether a resend is safe

"State unknown" is true and useless on its own. The caller's real question
is whether re-running recovers a lost run or pays for a second render, and
the CLI's own silence proves nothing: an empty stream looks the same
whether the task was never dispatched, the session id was wrong, or the
connection dropped.

Before returning 6, `create` and `video wait` now read the work row and put
the verdict in `error.detail`: `delivery` is `submitted` /
`not_submitted` / `indeterminate`, with a `resend_safe` boolean, the
backend's own status, and a `next_actions` command. `indeterminate`
reports `resend_safe: false` — waiting when a run was lost costs a delay,
resending when it was not costs the user's money.

### Changed — the run ledger falls back to the account

The ledger is per-machine, so "not recorded here" was a weak answer for an
agent that moved hosts, a rebuilt container, or a colleague picking the
work up. When the ledger has nothing, the video commands now resolve the
account's most recent run and say so on stderr.

Only for the no-arguments case: the backend's work list is keyed by
`work_id` and `session_id` and carries no `task_id`, so `vk video status
12345` against an empty ledger still exits 2 — pointing at `vk video list`
rather than pretending to look up something it cannot.

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
