# vibeknow-doc Command Reference

## Global Flags

| Flag | Description |
|------|-------------|
| `--output string` | Output format: `text\|json` (auto-selects based on TTY) |
| `--profile string` | Override active profile for this command |
| `--verbose` | Emit request/response summaries (credentials redacted) |

---

## doc upload

Upload a document to vectoria. Creates a knowledge base, polls until processing completes, and returns doc_id + kb_id.

```
vibeknow doc upload <file> [flags]
```

No command-specific flags.

**Arguments:**
- `<file>` — Path to local file to upload **(required)**

**Text output example:**
```
Uploading report.pdf...
Processing... done (12s)
doc_id: doc_abc123
kb_id:  kb_xyz789
```

**JSON output example:**
```json
{"doc_id":"doc_abc123","kb_id":"kb_xyz789","status":"completed"}
```

## doc get

Fetch document status from vectoria.

```
vibeknow doc get <doc_id> [flags]
```

| Flag | Description |
|------|-------------|
| `--kb-id string` | Knowledge base ID **(required)** |

**Arguments:**
- `<doc_id>` — Document ID **(required)**

**JSON output example:**
```json
{"doc_id":"doc_abc123","kb_id":"kb_xyz789","status":"completed"}
```

Possible `status` values: `processing`, `completed`, `failed`.
