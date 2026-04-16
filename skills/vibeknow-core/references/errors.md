# vibeknow-core Error Reference

## Exit Codes

| Exit Code | Meaning | Agent Action |
|-----------|---------|--------------|
| 0 | Success | — |
| 1 | General error | Read stderr for details |
| 2 | Invalid arguments | Fix command syntax, check `--help` |
| 3 | Auth error | Run `vibeknow auth status` to check credential source. If keychain/file: `vibeknow auth logout` and re-authenticate. If env var: check `VIBEKNOW_TOKEN` value. |
| 130 | User interrupt (SIGINT) | — |

Exit codes 4, 5, 6 are task-lifecycle codes and do not apply to core commands.

## Error Object (--output json)

When `--output json` is set, errors are returned as:

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

## Error Codes (relevant to core)

| Code | Meaning | Typical Scenario |
|------|---------|-----------------|
| `auth_required` | No credential found | No token in env, keychain, or file |
| `auth_expired` | Token has expired | Re-authenticate |
| `invalid_args` | Bad arguments | Wrong flag or missing required arg |
| `network_error` | Cannot reach endpoint | Check network, run `vibeknow doctor` |
| `version_mismatch` | CLI/server version incompatible | Update CLI |
| `internal_error` | Unexpected error | Report bug |
| `unknown` | Unclassified error | Read message field |
