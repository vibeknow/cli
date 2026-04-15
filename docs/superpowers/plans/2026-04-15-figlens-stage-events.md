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

Helper functions — add them in the **same file** (`base.go`) so this commit compiles standalone. English defaults inline; Task 3 will optionally upgrade to i18n later:

```go
func nodeStartMessage(stepID string) string   { return fmt.Sprintf("node %s: started", stepID) }
func nodeSuccessMessage(stepID string) string { return fmt.Sprintf("node %s: completed", stepID) }
func nodeErrorMessage(stepID string) string   { return fmt.Sprintf("node %s: failed", stepID) }
```

Make sure `"fmt"` is in the imports of `base.go` (likely already is).

- [ ] **Step 7: Commit**

```bash
cd ~/laoshen/go-figlens
git add internal/pipeline/node/base.go
git commit -m "feat(pipeline): emit start/success/error process events from runNodeWithRetry"
```

---

## Task 2: Update 14 node `Run()` methods to emit lifecycle events

**Verified against `~/laoshen/go-figlens` on 2026-04-15:**

11 of 14 nodes already use `runNodeWithRetry`; 3 nodes (`tts_generate`, `scene_generate`, `bg_images`) do **not** — they call external services where the current design intentionally skips the common retry wrapper. The plan adds a second helper `emitNodeLifecycle` for these 3, preserving their existing no-retry behavior.

**Actual file → node-type → stepID mapping** (copy-paste; do NOT re-infer):

| File | Go type name | Canonical stepID | Currently uses `runNodeWithRetry`? |
|---|---|---|---|
| `video_prepare.go` | `VideoPrepareNode` | `prepare` | ✅ |
| `video_knowledge.go` | `VideoKnowledgeNode` | `knowledge_detail` | ✅ |
| `video_speech.go` | `VideoSpeechNode` | `text_speech` | ✅ |
| `video_content_analyze.go` | `VideoContentAnalyzeNode` | `content_analyze` | ✅ (+ has pre-existing inline emit to delete) |
| `video_theme.go` | `VideoThemeNode` | `theme_select` | ✅ |
| `video_design.go` | `VideoDesignNode` | `design` | ✅ |
| `video_tts.go` | `VideoTTSNode` | `tts_generate` | ❌ — use `emitNodeLifecycle` |
| `video_scene.go` | `SceneGenerateNode` | `scene_generate` | ❌ — use `emitNodeLifecycle` |
| `video_bg_image.go` | `VideoBGImageNode` | `bg_images` | ❌ — use `emitNodeLifecycle` |
| `video_cover.go` | `VideoCoverNode` | `cover` | ✅ |
| `video_bgm.go` | `VideoBGMNode` | `bgm` | ✅ |
| `video_suggest.go` | `VideoSuggestNode` | `suggest` | ✅ |
| `video_package.go` | `VideoPackageNode` | `video_package` | ✅ |
| `video_finish.go` | `VideoFinishNode` | `video_finish` | ✅ |

- [ ] **Step 1: Add `emitNodeLifecycle` helper to `internal/pipeline/node/base.go`**

Place alongside the existing `runNodeWithRetry`. Same emit semantics but without retry:

```go
// emitNodeLifecycle wraps fn with start/success/error process event emission.
// Unlike runNodeWithRetry, it does NOT retry on failure — intended for nodes
// that call external services where retry is handled elsewhere (or undesired).
func emitNodeLifecycle(ctx context.Context, nodeName, stepID string, fn func() (any, error)) (any, error) {
	persistPipelineProcessLog(ctx, nodeStartMessage(stepID), "start", stepID)
	result, err := fn()
	if err != nil {
		pipelineErrorf(ctx, "节点 %s 失败: %v", nodeName, err)
		persistPipelineProcessLog(ctx, fmt.Sprintf("%s: %v", nodeErrorMessage(stepID), err), "error", stepID)
		return nil, err
	}
	pipelineInfos(ctx, "完成节点 %s", nodeName)
	persistPipelineProcessLog(ctx, nodeSuccessMessage(stepID), "success", stepID)
	return result, nil
}
```

- [ ] **Step 2: Update the 11 nodes that already use `runNodeWithRetry`**

For each, add the canonical stepID as the new second argument. Example for `video_prepare.go:37`:

Before:
```go
return runNodeWithRetry(ctx, "VideoPrepareNode", func() (any, error) { ... })
```

After:
```go
return runNodeWithRetry(ctx, "VideoPrepareNode", "prepare", func() (any, error) { ... })
```

Per-file edits (exact line numbers verified on 2026-04-15):

| File | Line | stepID to pass |
|---|---|---|
| `video_prepare.go:37` | `"prepare"` |
| `video_knowledge.go:28` | `"knowledge_detail"` |
| `video_speech.go:44` | `"text_speech"` |
| `video_content_analyze.go:37` | `"content_analyze"` |
| `video_theme.go:253` | `"theme_select"` |
| `video_design.go:153` | `"design"` |
| `video_cover.go:23` | `"cover"` |
| `video_bgm.go:28` | `"bgm"` |
| `video_suggest.go:22` | `"suggest"` |
| `video_package.go:72` | `"video_package"` |
| `video_finish.go:19` | `"video_finish"` |

- [ ] **Step 3: Wrap the 3 retry-free nodes with `emitNodeLifecycle`**

For `video_tts.go`, `video_scene.go`, `video_bg_image.go`:

Before (example `video_tts.go`):
```go
func (n *VideoTTSNode) Run(ctx context.Context, state graph.State) (any, error) {
    // ... existing logic ...
    return newState, nil
}
```

After:
```go
func (n *VideoTTSNode) Run(ctx context.Context, state graph.State) (any, error) {
    return emitNodeLifecycle(ctx, "VideoTTSNode", "tts_generate", func() (any, error) {
        // ... existing logic ...
        return newState, nil
    })
}
```

Apply the same pattern to:
- `video_scene.go` → `emitNodeLifecycle(ctx, "SceneGenerateNode", "scene_generate", ...)`
- `video_bg_image.go` → `emitNodeLifecycle(ctx, "VideoBGImageNode", "bg_images", ...)`

- [ ] **Step 4: Remove the pre-existing inline emit in `video_content_analyze.go`**

`VideoContentAnalyzeNode.Run()` currently emits its own start/success events manually. With the wrapper emitting the same events, delete these 2 lines to avoid double-emit. Search in that file for `persistPipelineProcessLog` and remove the 2 calls.

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

If the `i18n.AssistantEventTextByProvider` / `i18n.KeyPipelineContentAnalyzeStart` imports are no longer used in this file, remove them.

- [ ] **Step 5: Grep to verify no stray emits**

```bash
grep -rn "persistPipelineProcessLog" internal/pipeline/node/
```

Should hit only:
- `internal/pipeline/node/base.go` (definition + `runNodeWithRetry` + `emitNodeLifecycle` usage)

No per-node manual call sites remain.

- [ ] **Step 6: Build**

```bash
go build ./...
```

All 14 files compile.

- [ ] **Step 7: Commit**

```bash
git add internal/pipeline/node/
git commit -m "refactor(pipeline): emit stage events from all 14 nodes (retry-wrapped + 3 retry-free)"
```

---

## Task 3: i18n messages (deferred — NOT needed for P2 CLI)

**Skip this task for the 4-hour budget.** Task 1 already ships English defaults via `nodeStartMessage/Success/Error` — sufficient for CLI consumption.

Later (when scheduling allows), upgrade to full i18n:
- Add keys for 14 nodes × 3 statuses = 42 new i18n keys (or parameterized keys)
- Replace the inline `fmt.Sprintf("node %s: started", stepID)` with `i18n.AssistantEventTextByProvider(..., i18n.KeyPipelineNodeStart, stepID)`

Not blocking P2 — CLI routes via `step_id` + `status` (machine-readable), not via message text.

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
