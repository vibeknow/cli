---
name: vibeknow-doc
description: "Upload documents to vectoria and check processing status. Use when: user wants to upload a document, check if a document is ready, or get a doc_id for use with vibeknow create."
version: 0.7.1
emoji: "📄"
homepage: https://github.com/vibeknow/cli
allowed-tools: Bash(vibeknow:*)
metadata:
  openclaw:
    requires:
      bins: ["vibeknow"]
    install:
      - kind: node
        package: vibeknow-cli
        bins: [vibeknow]
    primaryEnv: VIBEKNOW_TOKEN
    envVars:
      - name: VIBEKNOW_TOKEN
        required: false
        description: "API token. Optional — if unset, the CLI uses credentials configured via `vibeknow auth login` (managed by vibeknow-core)."
---

# vibeknow-doc

## TRIGGER

- Upload a document to vectoria for processing
- Check document processing status
- Get a doc_id for later use with `vibeknow create --from <doc_id>`

## SKIP

- Video generation or task management → use **vibeknow-create**
- Auth, profile, environment setup → use **vibeknow-core**
- RAG queries → not yet available

## Core Concepts

- **doc upload**: Uploads a local file to vectoria. Creates a knowledge base, polls until processing completes, and returns the `doc_id` + `kb_id`.
- **doc get**: Fetches document status from vectoria. Requires both `<doc_id>` and `--kb-id`.
- **Document states**: `processing` → `completed` or `failed`.
- **Relationship to create**: `vibeknow create --from <doc_id>` uses an already-uploaded document. If you pass a file path or URL to `create --from`, it uploads automatically — you don't need `doc upload` first.

## Quick Reference

| Command | Description |
|---------|-------------|
| `vibeknow doc upload <file>` | Upload a document to vectoria |
| `vibeknow doc get <doc_id> --kb-id <kb_id>` | Fetch document status |

For full flags and output examples, see [commands.md](references/commands.md).

## Common Tasks

### Upload a document and get its doc_id

```bash
vibeknow doc upload ./report.pdf
# Output includes doc_id and kb_id
```

```bash
vibeknow doc upload ./report.pdf --output json
# {"doc_id":"doc_abc123","kb_id":"kb_xyz789","status":"completed"}
```

### Check if a document is ready

```bash
vibeknow doc get doc_abc123 --kb-id kb_xyz789
```

### Upload then create video (two-step)

```bash
# Step 1: upload (useful if you want to reuse the doc_id)
result=$(vibeknow doc upload ./slides.pdf --output json)
doc_id=$(echo "$result" | jq -r '.doc_id')

# Step 2: create video from the uploaded doc
vibeknow create --from "$doc_id"
```

**Note:** For one-shot workflows, `vibeknow create --from ./slides.pdf` does both steps automatically.

## Exit Code Handling

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | — |
| 1 | General error | Read stderr |
| 2 | Invalid arguments | Check file path exists, doc_id format |
| 3 | Auth error | Run `vibeknow auth status` to inspect credential source. Re-login with `vibeknow auth login` (interactive) or set `VIBEKNOW_TOKEN`. See **vibeknow-core** for profile/diagnostics if installed. |
| 130 | User interrupt | — |

For full error reference, see [errors.md](references/errors.md).

## Output Formats

```bash
vibeknow doc upload ./report.pdf                   # text (human-readable)
vibeknow doc upload ./report.pdf --output json      # structured JSON
vibeknow doc get doc_abc --kb-id kb_xyz --output json
```

## References

- [commands.md](references/commands.md) — Full flag reference
- [errors.md](references/errors.md) — Exit codes and error codes
