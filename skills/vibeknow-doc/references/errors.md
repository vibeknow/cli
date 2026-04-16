# vibeknow-doc Error Reference

## Exit Codes

| Exit Code | Meaning | Agent Action |
|-----------|---------|--------------|
| 0 | Success | — |
| 1 | General error | Read stderr. May indicate vectoria processing failure. |
| 2 | Invalid arguments | Check that file path exists, is a regular file, and is under 500MB size limit. Check doc_id and kb_id formats. |
| 3 | Auth error | Credential missing or expired. Use vibeknow-core skill to diagnose. |
| 130 | User interrupt (SIGINT) | Upload may have partially completed. |

Exit codes 4, 5, 6 are task-lifecycle codes and do not apply to doc commands.

## Error Object (--output json)

```json
{
  "schema_version": "1",
  "error": {
    "code": "<error_code>",
    "message": "human-readable message",
    "details": {},
    "retryable": false
  }
}
```

## Error Codes (relevant to doc)

| Code | Meaning | Typical Scenario |
|------|---------|-----------------|
| `auth_required` | No credential | Token missing |
| `auth_expired` | Token expired | Re-authenticate |
| `invalid_args` | Bad arguments | Missing file path, invalid doc_id |
| `not_found` | Resource not found | Bad doc_id or kb_id |
| `network_error` | Cannot reach vectoria | Check connectivity, run `vibeknow doctor` |
| `internal_error` | Server error | Report bug |
