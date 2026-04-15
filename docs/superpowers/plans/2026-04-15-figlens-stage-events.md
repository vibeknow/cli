# go-figlens: Emit Stage Events from Pipeline Runner

> **Scope:** Backend-only mini-plan for `go-figlens` (not for vibeknow-cli itself). Unblocks P2 of the CLI (see `docs/superpowers/specs/2026-04-15-vibeknow-cli-design.md` §4.1.2 + §5.3).
>
> **For agentic workers:** Optional. This plan is small enough to execute by hand in ~4 hours. If you do delegate, REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`.

**Goal:** Make `go-figlens`'s pipeline runner emit structured `process` events (with `step_id` = canonical node name + `status` = `start`/`success`/`error`) for every one of the 14 pipeline nodes, so clients subscribing to `/v1/agent3forVideo/stream` can derive stage progress without parsing log messages.

**Architecture:** Move emit logic into the existing universal node wrapper `runNodeWithRetry()` at `internal/pipeline/node/base.go:264`. Remove the one-off existing emit in `video_content_analyze.go` to avoid double-emit. No DB migration, no API schema changes.

**Tech Stack:** Go. Existing go-figlens packages: `internal/pipeline/node/`, `internal/pipeline/video_pipeline.go`, `internal/model/chat.go`, `internal/i18n/`.

**Est. effort:** ~4 hours.

---

## Work repo

All changes in `~/laoshen/go-figlens`. Branch: up to you (direct main or feature branch).

## What this plan does NOT change

- `AssistantEvent` / `AssistantLog` schema — fields already exist, just populating consistently.
- DB schema — no migration.
- API endpoints — no new routes or response schema changes.
- SSE handler — consumer gets more events; wire format unchanged.
- Billing, post-editing, agent mode (`agent2forVideo`) — untouched.

---

## Canonical node → stage mapping (reference)

Used in this plan and later mirrored in CLI (vibeknow-cli P2):

| Stage (logical, 6) | Nodes (14 total) |
|---|---|
| `parse` | `prepare`, `knowledge_detail`, `text_speech`, `content_analyze` |
| `outline` | `theme_select`, `design` |
| `tts` | `tts_generate` |
| `render` | `scene_generate`, `bg_images`, `cover`, `bgm` |
| `publish` | `video_package`, `video_finish` |
| `suggest` | `suggest` |

The backend does NOT need to encode stages — it emits **node-level** events. CLI does the node→stage grouping.

---

## Task 1: Refactor `runNodeWithRetry` to emit start/success/error events

**Files:**
- Modify: `internal/pipeline/node/base.go:264-292`

- [ ] **Step 1: Read the current `runNodeWithRetry` implementation**

Current signature:
```go
func runNodeWithRetry(ctx context.Context, nodeName string, fn func() (any, error)) (any, error)
```

The `nodeName` parameter is the Go type name (`"VideoContentAnalyzeNode"`), used for logging. This is NOT the same as the canonical `step_id` (`"content_analyze"` from `sg.AddNode(...)`). Need a separate param.

- [ ] **Step 2: Add a `stepID` parameter**

New signature:
```go
// runNodeWithRetry wraps a node's execution with retries and emits
// start/success/error process events keyed by stepID (the canonical name
// matching sg.AddNode, e.g. "content_analyze").
func runNodeWithRetry(ctx context.Context, nodeName, stepID string, fn func() (any, error)) (any, error)
```

- [ ] **Step 3: Emit `start` event immediately before the first attempt**

```go
persistPipelineProcessLog(ctx, nodeStartMessage(stepID), "start", stepID)
```

Where `nodeStartMessage` returns an i18n message for the node (see Task 4). If i18n isn't wired yet for a particular node, fallback to the node name itself.

- [ ] **Step 4: Emit `success` event on final success**

Inside the success branch (`err == nil`), AFTER `pipelineInfos(ctx, "完成节点 %s", nodeName)`:
```go
persistPipelineProcessLog(ctx, nodeSuccessMessage(stepID), "success", stepID)
```

- [ ] **Step 5: Emit `error` event when retries are exhausted**

At the point where `attempt == defaultNodeMaxAttempts` (terminal failure):
```go
persistPipelineProcessLog(ctx, fmt.Sprintf("%s: %v", nodeErrorMessage(stepID), lastErr), "error", stepID)
```

Do NOT emit on intermediate retry failures (to keep the event stream concise; log messages still go to the server log).

- [ ] **Step 6: Full text of the new function**

```go
func runNodeWithRetry(ctx context.Context, nodeName, stepID string, fn func() (any, error)) (any, error) {
	persistPipelineProcessLog(ctx, nodeStartMessage(stepID), "start", stepID)

	var lastErr error
	for attempt := 1; attempt <= defaultNodeMaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			if attempt > 1 {
				pipelineInfos(ctx, "节点 %s 重试成功，第%d次尝试", nodeName, attempt)
			}
			pipelineInfos(ctx, "完成节点 %s", nodeName)
			persistPipelineProcessLog(ctx, nodeSuccessMessage(stepID), "success", stepID)
			return result, nil
		}
		lastErr = err
		if attempt == defaultNodeMaxAttempts {
			pipelineErrorf(ctx, "节点 %s 重试耗尽，最终失败: %v", nodeName, err)
			break
		}
		pipelineErrorf(ctx, "节点 %s 执行失败，准备重试，第%d/%d次: %v", nodeName, attempt, defaultNodeMaxAttempts, err)
		// existing backoff logic untouched
	}
	persistPipelineProcessLog(ctx, fmt.Sprintf("%s: %v", nodeErrorMessage(stepID), lastErr), "error", stepID)
	return nil, lastErr
}
```

Helper functions for Task 4 (defined next):
```go
func nodeStartMessage(stepID string) string   { return i18nNodeMessage(stepID, "start") }
func nodeSuccessMessage(stepID string) string { return i18nNodeMessage(stepID, "success") }
func nodeErrorMessage(stepID string) string   { return i18nNodeMessage(stepID, "error") }
```

- [ ] **Step 7: Commit**

```bash
cd ~/laoshen/go-figlens
git add internal/pipeline/node/base.go
git commit -m "feat(pipeline): emit start/success/error process events from runNodeWithRetry"
```

---

## Task 2: Update all 14 node Run() methods to pass `stepID`

**Files:**
- Modify: `internal/pipeline/node/video_prepare.go`
- Modify: `internal/pipeline/node/video_knowledge_detail.go`
- Modify: `internal/pipeline/node/video_text_speech.go`
- Modify: `internal/pipeline/node/video_content_analyze.go`
- Modify: `internal/pipeline/node/video_theme.go`
- Modify: `internal/pipeline/node/video_design.go`
- Modify: `internal/pipeline/node/video_tts_generate.go`
- Modify: `internal/pipeline/node/video_scene_generate.go`
- Modify: `internal/pipeline/node/video_bg_images.go`
- Modify: `internal/pipeline/node/video_cover.go`
- Modify: `internal/pipeline/node/video_bgm.go`
- Modify: `internal/pipeline/node/video_suggest.go`
- Modify: `internal/pipeline/node/video_package.go`
- Modify: `internal/pipeline/node/video_finish.go`

(File names are inferred from node purposes; verify against actual `internal/pipeline/node/` directory before editing.)

- [ ] **Step 1: Discover actual file names**

```bash
ls internal/pipeline/node/
```

Map each of the 14 canonical node identifiers (`prepare`, `knowledge_detail`, etc.) to its Go file. Make a note.

- [ ] **Step 2: For each of the 14 nodes, update the `runNodeWithRetry` call**

Change:
```go
return runNodeWithRetry(ctx, "VideoPrepareNode", func() (any, error) { ... })
```

to:
```go
return runNodeWithRetry(ctx, "VideoPrepareNode", "prepare", func() (any, error) { ... })
```

Node-identifier mapping (copy-paste friendly — these MUST match exactly the strings in `video_pipeline.go:169-189`'s `sg.AddNode(...)` calls):

| Go type name (existing `nodeName` arg) | Canonical `stepID` (new arg) |
|---|---|
| `VideoPrepareNode` | `prepare` |
| `VideoKnowledgeDetailNode` | `knowledge_detail` |
| `VideoTextSpeechNode` | `text_speech` |
| `VideoContentAnalyzeNode` | `content_analyze` |
| `VideoThemeSelectNode` | `theme_select` |
| `VideoDesignNode` | `design` |
| `VideoTTSGenerateNode` | `tts_generate` |
| `VideoSceneGenerateNode` | `scene_generate` |
| `VideoBGImagesNode` | `bg_images` |
| `VideoCoverNode` | `cover` |
| `VideoBGMNode` | `bgm` |
| `VideoSuggestNode` | `suggest` |
| `VideoPackageNode` | `video_package` |
| `VideoFinishNode` | `video_finish` |

Verify by comparing to `video_pipeline.go:169-189` — canonical strings take priority if anything differs.

- [ ] **Step 3: Remove the existing one-off emit in `video_content_analyze.go`**

This node currently calls `persistPipelineProcessLog` manually to avoid double-emit. Find and delete the two lines (search for `persistPipelineProcessLog` in `video_content_analyze.go`):

Before:
```go
return runNodeWithRetry(ctx, "VideoContentAnalyzeNode", func() (any, error) {
    persistPipelineProcessLog(ctx, i18n.AssistantEventTextByProvider("", i18n.KeyPipelineContentAnalyzeStart), "start", "content_analyze")
    // ... execution ...
    persistPipelineProcessLog(ctx, i18n.AssistantEventTextByProvider("", i18n.KeyPipelineContentAnalyzeDone), "success", "content_analyze")
    return graph.State{...}, nil
})
```

After:
```go
return runNodeWithRetry(ctx, "VideoContentAnalyzeNode", "content_analyze", func() (any, error) {
    // ... execution ...
    return graph.State{...}, nil
})
```

- [ ] **Step 4: Grep the codebase to confirm no stray `persistPipelineProcessLog` calls remain**

```bash
grep -rn "persistPipelineProcessLog" internal/pipeline/node/
```

Should only hit the helper definition itself, no per-node call sites (emit is now centralized in `runNodeWithRetry`).

- [ ] **Step 5: Build**

```bash
go build ./...
```

All 14 files compile.

- [ ] **Step 6: Commit**

```bash
git add internal/pipeline/node/
git commit -m "refactor(pipeline): thread canonical stepID through all 14 nodes; remove one-off emit in content_analyze"
```

---

## Task 3: i18n messages (optional — can default to English)

**Files:**
- Optional: `internal/i18n/` (add keys for 14 nodes × 3 statuses = 42 keys)
- Or: inline English defaults in `internal/pipeline/node/base.go`

- [ ] **Step 1: Pragmatic choice — inline English defaults**

For the 4-hour budget, skip full i18n for the new messages; use English placeholders that mention the stepID. This is acceptable because `process` event messages are primarily consumed by the CLI (which has its own i18n) and server logs.

Add to `internal/pipeline/node/base.go` (or a new `internal/pipeline/node/messages.go`):

```go
func i18nNodeMessage(stepID, status string) string {
	switch status {
	case "start":
		return fmt.Sprintf("node %s: started", stepID)
	case "success":
		return fmt.Sprintf("node %s: completed", stepID)
	case "error":
		return fmt.Sprintf("node %s: failed", stepID)
	default:
		return fmt.Sprintf("node %s: %s", stepID, status)
	}
}
```

- [ ] **Step 2 (skip for 4-hour budget)**: the richer i18n keys, once added, can replace the inline defaults. Ticket for later, not this mini-plan.

- [ ] **Step 3: Commit**

```bash
git add internal/pipeline/node/
git commit -m "feat(pipeline): node event message helper (English defaults; i18n TODO)"
```

---

## Task 4: Manual SSE verification

No unit test — this is a behavioral contract check against a live figlens instance.

- [ ] **Step 1: Start figlens locally**

```bash
cd ~/laoshen/go-figlens
# however you normally run it; e.g.
go run . serve
```

Or whichever entry point is standard for local dev.

- [ ] **Step 2: Submit a task via the existing flow**

Use the frontend, an existing curl recipe, or `vibeknow-cli` (once P2 is done, but for now likely the frontend is easier) to kick off a pipeline-mode task (`v=3`). Note the `session_id`.

- [ ] **Step 3: Subscribe to the SSE stream and tail events**

```bash
curl -N \
  -H "Authorization: Bearer <dev-token>" \
  -H "Accept: text/event-stream" \
  "http://127.0.0.1:20067/v1/agent3forVideo/stream?session_id=<session_id>"
```

Pipe to `jq` if desired:
```bash
curl -N ... | grep '^data:' | sed 's/^data: //' | jq -c 'select(.data.type == "process")'
```

- [ ] **Step 4: Acceptance criteria**

Over the lifetime of one successful task, verify:

1. Exactly **28 `process` events** appear (14 nodes × 2: start + success).
2. Each event's `log.step_id` is one of the 14 canonical node names.
3. Each event's `log.status` is either `start` or `success`.
4. Events arrive in the node execution order (roughly: prepare → knowledge_detail → ... → video_finish → suggest).
5. No duplicate (stepID, status) pairs within one session.

If you intentionally break a node (e.g., stop the knowledge service mid-task) to test the error path:

6. A `process` event with `status=error` appears for the failed node, and the rest of the pipeline aborts.

- [ ] **Step 5: Write a short confirmation note**

In `~/laoshen/vibeknow-cli/docs/contracts/p1-backend.md`, append a new section:

```markdown
## 7. figlens pipeline stage events (P2-prereq confirmation)

Verified 2026-04-15 against go-figlens commit <sha>:

- `/v1/agent3forVideo/stream` emits `process` AssistantEvents for each of 14 pipeline nodes.
- `log.step_id` ∈ {prepare, knowledge_detail, text_speech, content_analyze, theme_select, design, tts_generate, scene_generate, bg_images, cover, bgm, video_package, video_finish, suggest}
- `log.status` ∈ {start, success, error}
- Order matches the DAG defined in `internal/pipeline/video_pipeline.go` buildGraph().
- SSE supports full replay from `lastID=0` on reconnect.
```

Commit that note:
```bash
cd ~/laoshen/vibeknow-cli
git add docs/contracts/
git commit -m "docs(p1): confirm figlens pipeline stage events contract"
git push origin main
```

---

## Task 5: Announce readiness for P2 CLI plan

This mini-plan produces the figlens-side prereq for vibeknow-cli P2. Once Task 4's acceptance criteria are met:

- [ ] Notify CLI plan author (you) that P2 prereq is ready.
- [ ] CLI P2 plan can then be written with concrete event shapes, using the 14 canonical node identifiers above.
- [ ] Roll the figlens change out to staging before P2 CLI integration tests run against real servers.

---

## Self-review (after execution)

1. Does every node still run correctly? (Pipeline should still complete successful tasks end-to-end.)
2. Are there any nodes outside these 14 that also use `runNodeWithRetry`? (If yes, either pass a reasonable stepID or accept they emit events too.)
3. Is `content_analyze` still working now that its inline emit is gone? (runNodeWithRetry-provided emit takes over.)
4. Does error-path emit actually fire for a killed node? (Task 4 Step 4 #6.)
5. Does this change the existing server log volume meaningfully? (Should only add ~28 log lines per task.)
6. Any hot path where `persistPipelineProcessLog` calls could dominate latency? (Should be negligible — single DB insert per node transition, 14 nodes per ~6-minute task.)

If all green: push, notify CLI side, proceed with vibeknow-cli P2 plan.
