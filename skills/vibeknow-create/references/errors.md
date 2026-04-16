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
| 6 | Stream interrupted, **task status unknown** | The SSE connection broke after 3 retries. The task may still be running on the server. Run `vibeknow video wait <task_id> --session-id <sid>` to reconnect. Do **NOT** re-submit — this would create a duplicate task. |
| 130 | User interrupt (SIGINT) | Task may still be running server-side. |

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
