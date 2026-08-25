# vibeknow-create Error Reference

## Exit Codes

| Exit Code | Meaning | Agent Action |
|-----------|---------|--------------|
| 0 | Success | Extract `video_url` from stdout (JSON) or last NDJSON `task.succeeded` event |
| 1 | General error | Read stderr for details |
| 2 | Invalid arguments | Check `--from` is provided, file exists, URL is valid |
| 3 | Auth error | Credential missing or expired. Use vibeknow-core skill to diagnose. |
| 4 | Task failed, **retryable** | Safe to re-submit. The `task.failed` event's `retryable` field is `true`. |
| 5 | Task failed, **not retryable** | Do not retry. Report `error_code` and `error_message` to user. |
| 6 | Stream interrupted, **task status unknown** | The task may still be running on the server. Read `error.detail.resend_safe` before doing anything (below). Unless it is `true`, reconnect with `vibeknow video wait` — do **not** re-submit, which would create a duplicate billed task. |
| 7 | Partial success: preview ready, MP4 render failed | Report the `share_url`; retry only the export. |
| 8 | Blocked on a user decision | Show the pending action, wait for the user, run its `resume_command`. |
| 130 | User interrupt (SIGINT) | Task may still be running server-side. |

## Exit 6: `error.detail`

Exit 6 carries a machine-readable verdict on the one question that matters
here — whether re-running recovers a lost run or pays for a second one.

```json
{ "ok": false,
  "error": {
    "type": "api", "code": 6,
    "message": "the generation stream ended before the task reached a terminal state…",
    "hint": "vk video wait 4242 --session-id s_x",
    "detail": {
      "delivery": "submitted",
      "resend_safe": false,
      "task_id": 4242,
      "session_id": "s_x",
      "backend_status": "running",
      "next_actions": [{ "command": "vk video wait 4242 --session-id s_x",
                         "purpose": "Reattach to the run the backend still has; do not start a new one" }]
    } } }
```

| `delivery` | `resend_safe` | Meaning |
|------------|---------------|---------|
| `submitted` | `false` | The backend has a work row for this session. It is likely still running. Reattach. |
| `not_submitted` | `true` | The backend returned not_found. The work row is created at init time, so its absence means nothing was dispatched and nothing was billed. Safe to start over. |
| `indeterminate` | `false` | The probe itself failed (network, auth, 5xx). Unknown is not permission — check with `vibeknow video list` first. |

`backend_status` is present only for `submitted`, and is one of `running`,
`succeeded`, `failed`, `deleted`, `unknown`.

## Exit 8: blocked on a decision

Written to **stdout**, because it is the command's result rather than an
error report:

```json
{ "status": "blocked",
  "pending_actions": [{
    "action_id": "act_9f3c1b…", "type": "export_confirmation", "blocking": true,
    "message": "About to render MP4 (~several minutes, consumes 1 export credit). Continue?",
    "payload": { "session_id": "s_x", "credits": 1, "operation": "render_mp4" },
    "options": [{ "id": "confirm", "effect": "resume", "label": "Proceed and spend the credits" },
                { "id": "cancel",  "effect": "none",   "label": "Do not proceed; run nothing" }],
    "resume_command": "vk video export 42 --session-id s_x --confirm act_9f3c1b…" }] }
```

Rules:

- Present `message` and `payload` to the user. Never default a choice.
- `effect: "resume"` → run `resume_command` **verbatim**.
- `effect: "none"` → run nothing.
- `action_id` is not derivable and is bound to this run and this price. A
  rejected token (exit 2) means the terms changed: re-run without
  `--confirm`, show the new terms, ask again.
- Do not reach for `--yes` to get past a block you just received.

## stage.failed Behavior

- `fatal=false`: Progress information only. The pipeline may retry internally or skip the stage. CLI continues streaming. No non-zero exit code is set from this event alone.
- `fatal=true`: The task is about to fail. CLI preserves the error info and exits with code 4 or 5 when `task.failed` arrives.
- If SSE disconnects after `fatal=true` stage.failed but before `task.failed`: CLI exits with code 6.

## Error Object (--output json)

When a command fails before a task is created (auth error, bad args, network error), the output is:

```json
{
  "schema_version": "1",
  "error": {
    "code": "<error_code>",
    "message": "human-readable message",
    "details": {},
    "retryable": false,
    "trace_id": "string (if VIBEKNOW_TRACE=1)"
  }
}
```

This is distinct from task events — it means no task was created at all.

## Error Codes (relevant to create)

| Code | Meaning | Typical Scenario |
|------|---------|-----------------|
| `auth_required` | No credential | Token missing |
| `auth_expired` | Token expired | Re-authenticate |
| `invalid_args` | Bad arguments | Missing `--from`, invalid file path |
| `not_found` | Resource not found | Bad doc_id or task_id |
| `network_error` | Cannot reach endpoint | Check connectivity |
| `stream_interrupted` | SSE connection lost | Maps to exit code 6 |
| `task_failed` | Task terminated with error | Check `retryable` field |
| `rate_limited` | Too many requests | Wait and retry |
| `internal_error` | Server error | Report bug |
