# P2: Shortcuts & Hero Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the hero `vibeknow create` command and supporting shortcuts (`doc upload/get`, `voice list`, `video status/wait/download`) so the CLI can turn a local file or URL into a video end-to-end. This is the first user-facing value beyond auth/config.

**Architecture:** Three new service clients (`client/vectoria`, `client/figlens`, `client/vibeknow`) on the P1 `httpclient` foundation. A new `internal/sse` package handles Server-Sent Events streaming from figlens. A new `internal/stage` package maps 14 figlens pipeline nodes to 6 logical stages. Shortcut commands in `shortcuts/` orchestrate multi-step flows; API-layer commands in `cmd/` expose single-call operations. The hero `create` shortcut chains: vectoria KB creation → doc upload → doc poll → figlens task init → SSE stream → work detail fetch. See spec §5-6, frontend flow doc.

**Tech Stack:** Go 1.25+, existing P1 packages (`internal/httpclient`, `internal/cliauth`, `internal/endpoints`, `internal/output`, `internal/errs`), `bufio.Scanner` for SSE line parsing, `mime/multipart` for file upload. No new external dependencies.

**Scope boundary (what P2 does NOT include):**
- `project create/use/list/show` — backend CRUD does not exist yet; deferred.
- `rag query` — vectoria RAG query; separate plan.
- `video cancel` — no figlens endpoint; deferred.
- `video edit` (post-editing) — separate plan per spec.
- Interactive login (`auth login`) — P1.5 scope.
- AI Skills (`skills/`) — P3 scope.
- `video retry` — depends on figlens retry API; deferred.
- `--project` flag on `create` — depends on project CRUD.

**Backend prerequisites (parallel track):**
- **P2 prereq (figlens):** Pipeline runner emits `process` events with `step_id = <node_name>` + `status = start|success|error` for all 14 nodes. See spec §4.1.2, figlens mini-plan. ~4h work, no DB migration.
- **vectoria API key:** CLI uses `VECTORIA_API_KEY` env var injected via `X-API-Key` header (not JWT). Confirm with vectoria owner.

---

## Backend contract: P2 additions

This section freezes the API shapes the CLI expects. Backend owners implement in parallel; CLI tests use `httptest` fakes.

### vectoria (rag service)

All vectoria requests use `X-API-Key` header (NOT `Authorization: Bearer`). JWT is not used.

| Method | Path | Request | Response (200) |
|--------|------|---------|----------------|
| POST | `/knowledgebases` | `{"name":"<auto>"}` | `{"id":"kb_xxx"}` |
| POST | `/knowledgebases/{kb_id}/documents/file` | multipart: `file` field | `{"id":"doc_xxx","status":"processing"}` |
| POST | `/knowledgebases/{kb_id}/documents/url` | `{"url":"https://..."}` | `{"id":"doc_xxx","status":"processing"}` |
| GET | `/knowledgebases/{kb_id}/documents/{doc_id}` | — | `{"id":"doc_xxx","status":"processing|completed|failed","error":"..."}` |
| DELETE | `/knowledgebases/{kb_id}/documents/{doc_id}` | — | 204 |

### figlens

| Method | Path | Request | Response |
|--------|------|---------|----------|
| POST | `/v1/tasks/init` | `{"v":3}` | `{"task_id":123,"session_id":"s_xxx","work_id":"w_xxx"}` |
| POST | `/v1/agent3forVideo/stream` | `{"query":"...","task_id":123,"session_id":"s_xxx","knowledge_id":"kb_xxx","doc_id":"doc_xxx","voice_id":"v_xxx"}` | SSE event stream |
| GET | `/v1/works/detailBySession?session_id=s_xxx` | — | `{"id":"w_xxx","title":"...","video_path":"...","cover_url":"...","duration":120}` |
| POST | `/v1/agent2forVideo/exportRemoteV2` | `{"session_id":"s_xxx"}` | `{"task_id":"export_xxx"}` |
| POST | `/v1/agent2forVideo/exportResultV2` | `{"task_id":"export_xxx"}` | `{"status":"processing|completed","video_path":"..."}` |
| POST | `/v1/agent2forVideo/signedUrl` | `{"path":"..."}` | `{"url":"https://signed..."}` |

### figlens SSE event shapes

Each SSE `data:` line is JSON: `{"code":200,"data":{"type":"<type>","log":{...}}}`.

| `data.type` | Key fields in `data.log` / `data` | CLI maps to |
|-------------|----------------------------------|-------------|
| `process` | `step_id`, `status` (start/success/error), `message` | `stage.started` / `stage.succeeded` / `stage.failed` |
| `data` | `answer_delta` (text chunk) | displayed in TTY, ignored in NDJSON |
| `aim_result` | `session_id`, completion signal | `task.succeeded` trigger |
| `suggest` | suggestions array | displayed in TTY, emitted as `task.suggest` event |
| `error` / `ERROR` | `message` | `task.failed` |
| `[DONE]` | — | stream close signal |

### vibeknow

| Method | Path | Request | Response |
|--------|------|---------|----------|
| GET | `/v1/voice-templates?page=1&size=100` | — | `{"items":[{"id":"v_xxx","name":"...","language":"...","gender":"...","preview_url":"..."}]}` |
| GET | `/v1/credits/balance` | — | `{"balance":500}` |

---

## File Structure (what this plan creates/modifies)

```
vibeknow-cli/
├── docs/
│   └── contracts/
│       └── p2-backend.md                  # T1: backend contract doc (extracted from this plan header)
├── internal/
│   ├── sse/                               # T2: NEW — SSE line parser
│   │   ├── reader.go                      #   Scanner-based SSE reader
│   │   └── reader_test.go
│   ├── stage/                             # T3: NEW — node→stage mapping
│   │   ├── stage.go                       #   14 nodes → 6 stages + types
│   │   └── stage_test.go
│   └── httpclient/
│       ├── client.go                      # T4: add DoRaw (returns *http.Response for SSE)
│       └── upload.go                      # T5: NEW — multipart file upload helper
├── client/
│   ├── vectoria/                          # T6: NEW
│   │   ├── client.go                      #   vectoria client (X-API-Key, not JWT)
│   │   ├── knowledgebase.go               #   CreateKB, UploadDoc, UploadURL, GetDocStatus, DeleteDoc
│   │   └── knowledgebase_test.go
│   ├── figlens/                           # T7–T8: NEW
│   │   ├── client.go                      #   figlens client
│   │   ├── task.go                        #   InitTask
│   │   ├── stream.go                      #   StreamChat (SSE)
│   │   ├── work.go                        #   GetWorkBySession
│   │   ├── export.go                      #   ExportVideo, GetExportResult, SignedURL
│   │   └── figlens_test.go
│   └── vibeknow/                          # T9: NEW
│       ├── client.go                      #   vibeknow client
│       ├── voice.go                       #   ListVoiceTemplates
│       └── voice_test.go
├── cmd/
│   ├── doc/                               # T10: NEW
│   │   ├── doc.go                         #   parent command
│   │   ├── upload.go                      #   `doc upload <file>`
│   │   └── get.go                         #   `doc get <id>`
│   ├── voice/                             # T11: NEW
│   │   ├── voice.go                       #   parent command
│   │   └── list.go                        #   `voice list`
│   ├── video/                             # T12: NEW
│   │   ├── video.go                       #   parent command
│   │   ├── status.go                      #   `video status <task_id>`
│   │   ├── wait.go                        #   `video wait <task_id>` (SSE reconnect)
│   │   └── download.go                    #   `video download <task_id>`
│   ├── create.go                          # T13: NEW — hero shortcut `vibeknow create`
│   └── root.go                            # T14: MODIFY — register new command groups
└── tests/
    └── integration/
        └── create_flow_test.go            # T15: end-to-end with httptest fakes
```

---

## Conventions (apply to every task)

- TDD: failing test first for every new logic.
- Commit after each task with Conventional Commits (`feat:`, `test:`, `refactor:`).
- All user-visible strings route through `internal/i18n`.
- HTTP client tests use `httptest.NewServer` — no real network.
- Exit codes (spec §5.4): 0=success, 2=invalid_args, 3=auth, 5=task_failed_fatal, 6=stream_interrupted.
- New service clients follow the `client/account` pattern: wrap `httpclient.Client`, use `StandardChain`.
- vectoria client uses a **custom chain** (X-API-Key instead of Bearer token).

---

## Task 1: P2 backend contract document

**Files:**
- Create: `docs/contracts/p2-backend.md`

Pure documentation: freezes the API shapes for vectoria, figlens, and vibeknow that P2 CLI expects.

- [ ] **Step 1: Write `docs/contracts/p2-backend.md`**

```markdown
# P2 backend contract (CLI expectations)

**Status:** DRAFT — awaiting sign-off from vectoria / go-figlens / go-vibeknow owners.
**Depends on:** P1 backend contract (error shape, health, api-version header).

## 1. vectoria — knowledge base & document APIs

Auth: `X-API-Key` header (NOT JWT Bearer). CLI reads from `VECTORIA_API_KEY` env var.

### POST /knowledgebases
Request: `{"name":"vibeknow-cli-<timestamp>"}`
Response 200: `{"id":"kb_xxx"}`

### POST /knowledgebases/{kb_id}/documents/file
Request: multipart/form-data with field `file`.
Response 200: `{"id":"doc_xxx","status":"processing"}`

### POST /knowledgebases/{kb_id}/documents/url
Request: `{"url":"https://example.com/article"}`
Response 200: `{"id":"doc_xxx","status":"processing"}`

### GET /knowledgebases/{kb_id}/documents/{doc_id}
Response 200: `{"id":"doc_xxx","status":"processing|completed|failed","error":"only if failed"}`

### DELETE /knowledgebases/{kb_id}/documents/{doc_id}
Response: 204 No Content.

## 2. figlens — task, stream, work, export APIs

Auth: `Authorization: Bearer <jwt>` (same as P1).

### POST /v1/tasks/init
Request: `{"v":3}`
Response 200: `{"code":200,"data":{"task_id":123,"session_id":"s_xxx","work_id":"w_xxx"}}`

### POST /v1/agent3forVideo/stream (SSE)
Request: `{"query":"...","task_id":123,"session_id":"s_xxx","knowledge_id":"kb_xxx","doc_id":"doc_xxx","voice_id":"v_xxx"}`
Response: SSE text/event-stream. Each `data:` line is JSON `{"code":200,"data":{"type":"<type>","log":{...}}}`.
Event types: process, data, aim_result, suggest, error, [DONE].

### GET /v1/works/detailBySession?session_id=s_xxx
Response 200: `{"code":200,"data":{"id":"w_xxx","title":"...","video_path":"...","cover_url":"...","duration":120}}`

### POST /v1/agent2forVideo/exportRemoteV2
Request: `{"session_id":"s_xxx"}`
Response 200: `{"code":200,"data":{"task_id":"export_xxx"}}`

### POST /v1/agent2forVideo/exportResultV2
Request: `{"task_id":"export_xxx"}`
Response 200: `{"code":200,"data":{"status":"processing|completed","video_path":"..."}}`

### POST /v1/agent2forVideo/signedUrl
Request: `{"path":"<video_path or html_path>"}`
Response 200: `{"code":200,"data":{"url":"https://signed-url..."}}`

## 3. vibeknow — voice templates

Auth: `Authorization: Bearer <jwt>`.

### GET /v1/voice-templates?page=1&size=100
Response 200: `{"code":200,"data":{"items":[{"id":"v_xxx","name":"...","language":"zh","gender":"female","preview_url":"..."}]}}`

## 4. Open questions for backend owners

1. vectoria: confirm doc_id format (regex `^doc_[a-zA-Z0-9]{8,}$`?).
2. vectoria: confirm X-API-Key is the correct auth header name.
3. figlens: confirm `/v1/agent3forVideo/stream` is the stable pipeline-mode endpoint.
4. figlens: confirm `task_id` from `/v1/tasks/init` is int (not string).
5. figlens: all responses wrapped in `{"code":200,"data":{...}}` — confirm this is consistent.
6. figlens: confirm SSE reconnect behavior — POST same endpoint with task_id+session_id replays all events.
```

- [ ] **Step 2: Commit**

```bash
git add docs/contracts/p2-backend.md
git commit -m "docs: add P2 backend contract for vectoria/figlens/vibeknow"
```

---

## Task 2: SSE reader (`internal/sse`)

**Files:**
- Create: `internal/sse/reader.go`
- Create: `internal/sse/reader_test.go`

A minimal, zero-dependency SSE line parser. Reads from `io.Reader`, yields `Event` structs. No reconnect logic here — that lives in `client/figlens/stream.go`.

- [ ] **Step 1: Write the failing test**

Create `internal/sse/reader_test.go`:

```go
package sse_test

import (
	"strings"
	"testing"

	"github.com/vibeknow/cli/internal/sse"
)

func TestReader_BasicEvents(t *testing.T) {
	input := "data: {\"type\":\"process\"}\n\ndata: {\"type\":\"done\"}\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != `{"type":"process"}` {
		t.Fatalf("got data %q, want process event", ev.Data)
	}

	ev, err = r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != `{"type":"done"}` {
		t.Fatalf("got data %q, want done event", ev.Data)
	}
}

func TestReader_MultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "line1\nline2" {
		t.Fatalf("got data %q, want multi-line concat", ev.Data)
	}
}

func TestReader_EventField(t *testing.T) {
	input := "event: error\ndata: {\"msg\":\"fail\"}\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "error" {
		t.Fatalf("got event %q, want error", ev.Event)
	}
}

func TestReader_IDField(t *testing.T) {
	input := "id: 42\ndata: hello\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "42" {
		t.Fatalf("got id %q, want 42", ev.ID)
	}
}

func TestReader_SkipsComments(t *testing.T) {
	input := ": keep-alive\ndata: real\n\n"
	r := sse.NewReader(strings.NewReader(input))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "real" {
		t.Fatalf("got data %q, want real", ev.Data)
	}
}

func TestReader_EOF(t *testing.T) {
	input := "data: last\n\n"
	r := sse.NewReader(strings.NewReader(input))

	_, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error on first: %v", err)
	}

	_, err = r.Next()
	if err == nil {
		t.Fatal("expected io.EOF, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/sse/... -v
```
Expected: compilation error — package `sse` does not exist.

- [ ] **Step 3: Write implementation**

Create `internal/sse/reader.go`:

```go
// Package sse provides a minimal Server-Sent Events reader per the W3C spec.
// It reads from an io.Reader and yields Event structs. No reconnect logic;
// callers handle that at a higher level.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// Event represents a single SSE event.
type Event struct {
	ID    string
	Event string
	Data  string
}

// Reader reads SSE events from a stream.
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader wraps an io.Reader as an SSE event reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{scanner: bufio.NewScanner(r)}
}

// Next returns the next complete event. Returns io.EOF when the stream ends
// with no pending event.
func (r *Reader) Next() (Event, error) {
	var ev Event
	var dataLines []string
	hasData := false

	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Empty line = event boundary.
		if line == "" {
			if hasData || ev.Event != "" || ev.ID != "" {
				ev.Data = strings.Join(dataLines, "\n")
				return ev, nil
			}
			continue
		}

		// Comment lines start with ':'.
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ") // single leading space is stripped per spec

		switch field {
		case "data":
			dataLines = append(dataLines, value)
			hasData = true
		case "event":
			ev.Event = value
		case "id":
			ev.ID = value
		// "retry" and unknown fields are ignored.
		}
	}

	if err := r.scanner.Err(); err != nil {
		return Event{}, err
	}

	// If we have a partial event at EOF, emit it.
	if hasData || ev.Event != "" || ev.ID != "" {
		ev.Data = strings.Join(dataLines, "\n")
		return ev, nil
	}
	return Event{}, io.EOF
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/sse/... -v
```
Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sse/
git commit -m "feat(sse): add minimal SSE reader for figlens event streaming"
```

---

## Task 3: Stage mapping (`internal/stage`)

**Files:**
- Create: `internal/stage/stage.go`
- Create: `internal/stage/stage_test.go`

Maps the 14 figlens pipeline nodes to 6 logical stages (spec §5.3). Pure data + lookup — no I/O.

- [ ] **Step 1: Write the failing test**

Create `internal/stage/stage_test.go`:

```go
package stage_test

import (
	"testing"

	"github.com/vibeknow/cli/internal/stage"
)

func TestNodeToStage(t *testing.T) {
	tests := []struct {
		node string
		want string
	}{
		{"prepare", "parse"},
		{"knowledge_detail", "parse"},
		{"text_speech", "outline"},
		{"content_analyze", "outline"},
		{"theme_select", "outline"},
		{"design", "outline"},
		{"tts_generate", "tts"},
		{"scene_generate", "render"},
		{"bg_images", "render"},
		{"cover", "render"},
		{"bgm", "render"},
		{"video_package", "publish"},
		{"video_finish", "publish"},
		{"suggest", "suggest"},
	}
	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			got, ok := stage.FromNode(tt.node)
			if !ok {
				t.Fatalf("node %q not found in mapping", tt.node)
			}
			if got != tt.want {
				t.Fatalf("FromNode(%q) = %q, want %q", tt.node, got, tt.want)
			}
		})
	}
}

func TestNodeToStage_Unknown(t *testing.T) {
	_, ok := stage.FromNode("nonexistent_node")
	if ok {
		t.Fatal("expected unknown node to return ok=false")
	}
}

func TestAllNodes(t *testing.T) {
	nodes := stage.AllNodes()
	if len(nodes) != 14 {
		t.Fatalf("expected 14 nodes, got %d", len(nodes))
	}
}

func TestStageOrder(t *testing.T) {
	stages := stage.OrderedStages()
	expected := []string{"parse", "outline", "tts", "render", "publish", "suggest"}
	if len(stages) != len(expected) {
		t.Fatalf("expected %d stages, got %d", len(expected), len(stages))
	}
	for i, s := range stages {
		if s != expected[i] {
			t.Fatalf("stage[%d] = %q, want %q", i, s, expected[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/stage/... -v
```
Expected: compilation error.

- [ ] **Step 3: Write implementation**

Create `internal/stage/stage.go`:

```go
// Package stage maps figlens pipeline node names to logical CLI stages.
// See spec §5.3: 14 nodes → 6 stages (parse/outline/tts/render/publish/suggest).
package stage

// nodeToStage maps each figlens pipeline node to its logical stage.
var nodeToStage = map[string]string{
	"prepare":          "parse",
	"knowledge_detail": "parse",
	"text_speech":      "outline",
	"content_analyze":  "outline",
	"theme_select":     "outline",
	"design":           "outline",
	"tts_generate":     "tts",
	"scene_generate":   "render",
	"bg_images":        "render",
	"cover":            "render",
	"bgm":              "render",
	"video_package":    "publish",
	"video_finish":     "publish",
	"suggest":          "suggest",
}

// orderedStages preserves the logical stage order for progress display.
var orderedStages = []string{"parse", "outline", "tts", "render", "publish", "suggest"}

// orderedNodes preserves the pipeline DAG execution order.
var orderedNodes = []string{
	"prepare", "knowledge_detail",
	"text_speech", "content_analyze", "theme_select", "design",
	"tts_generate",
	"scene_generate", "bg_images", "cover", "bgm",
	"video_package", "video_finish",
	"suggest",
}

// FromNode returns the logical stage for a pipeline node name.
func FromNode(node string) (string, bool) {
	s, ok := nodeToStage[node]
	return s, ok
}

// AllNodes returns the 14 pipeline node names in DAG execution order.
func AllNodes() []string {
	out := make([]string, len(orderedNodes))
	copy(out, orderedNodes)
	return out
}

// OrderedStages returns the 6 logical stages in display order.
func OrderedStages() []string {
	out := make([]string, len(orderedStages))
	copy(out, orderedStages)
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/stage/... -v
```
Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stage/
git commit -m "feat(stage): add figlens pipeline node-to-stage mapping (14→6)"
```

---

## Task 4: Extend `httpclient.Client` with `DoRaw`

**Files:**
- Modify: `internal/httpclient/client.go`
- Create: `internal/httpclient/client_raw_test.go`

SSE streaming needs the raw `*http.Response` (body stays open). Add `DoRaw` that returns `*http.Response` after going through the middleware chain, without reading/closing the body.

- [ ] **Step 1: Write the failing test**

Create `internal/httpclient/client_raw_test.go`:

```go
package httpclient_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/internal/httpclient"
)

func TestDoRaw_ReturnsOpenBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: hello\n\n"))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)
	resp, err := c.DoRaw(context.Background(), "POST", "/stream", map[string]any{"q": "test"})
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "data: hello\n\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestDoRaw_Returns4xxAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		w.Write([]byte(`{"code":40101,"message":"unauthorized"}`))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)
	_, err := c.DoRaw(context.Background(), "POST", "/stream", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestDoRaw -v
```
Expected: compilation error — `DoRaw` undefined.

- [ ] **Step 3: Write implementation**

Add to `internal/httpclient/client.go`:

```go
// DoRaw sends a request through the middleware chain and returns the raw
// *http.Response with body still open. Caller MUST close resp.Body.
// Returns an error for HTTP >= 400 (body is read and closed in that case).
// Use this for SSE streaming where the body is consumed incrementally.
func (c *Client) DoRaw(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("httpclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("httpclient: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		var eo *errObject
		if errors.As(err, &eo) {
			return nil, eo
		}
		return nil, &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, parseBackendError(resp)
	}
	return resp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestDoRaw -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpclient/client.go internal/httpclient/client_raw_test.go
git commit -m "feat(httpclient): add DoRaw for SSE streaming (returns open response body)"
```

---

## Task 5: Multipart upload helper (`internal/httpclient/upload.go`)

**Files:**
- Create: `internal/httpclient/upload.go`
- Create: `internal/httpclient/upload_test.go`

vectoria `doc upload` requires multipart/form-data. This helper builds the multipart body and sends it through the middleware chain.

- [ ] **Step 1: Write the failing test**

Create `internal/httpclient/upload_test.go`:

```go
package httpclient_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/internal/httpclient"
)

func TestDoUpload(t *testing.T) {
	var gotContentType string
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"doc_abc","status":"processing"}`))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)

	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err := c.DoUpload(context.Background(), "/upload", "file", "test.pdf",
		strings.NewReader("fake-pdf-content"), &out)
	if err != nil {
		t.Fatalf("DoUpload: %v", err)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart/form-data", gotContentType)
	}
	if gotBody != "fake-pdf-content" {
		t.Fatalf("file body = %q", gotBody)
	}
	if out.ID != "doc_abc" {
		t.Fatalf("response id = %q", out.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestDoUpload -v
```
Expected: compilation error — `DoUpload` undefined.

- [ ] **Step 3: Write implementation**

Create `internal/httpclient/upload.go`:

```go
package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// DoUpload sends a multipart/form-data POST with a single file field.
// fieldName is the form field name, fileName is the filename in the part header,
// fileBody is the file content reader. Response JSON is decoded into out.
func (c *Client) DoUpload(ctx context.Context, path, fieldName, fileName string, fileBody io.Reader, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	// Write multipart body in a goroutine to avoid buffering entire file.
	go func() {
		part, err := mw.CreateFormFile(fieldName, fileName)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, fileBody); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(mw.Close())
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, pr)
	if err != nil {
		return fmt.Errorf("httpclient: new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		var eo *errObject
		if errors.As(err, &eo) {
			return eo
		}
		return &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseBackendError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &errObject{Code: "unknown", Message: "decode response: " + err.Error()}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestDoUpload -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpclient/upload.go internal/httpclient/upload_test.go
git commit -m "feat(httpclient): add DoUpload for multipart file uploads"
```

---

## Task 6: vectoria client (`client/vectoria`)

**Files:**
- Create: `client/vectoria/client.go`
- Create: `client/vectoria/knowledgebase.go`
- Create: `client/vectoria/knowledgebase_test.go`

vectoria uses `X-API-Key` auth, not JWT Bearer. Needs a custom middleware chain.

- [ ] **Step 1: Write the failing test**

Create `client/vectoria/knowledgebase_test.go`:

```go
package vectoria_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/vectoria"
)

func TestCreateKB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/knowledgebases" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing X-API-Key header")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] == "" {
			t.Fatal("expected name in body")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "kb_abc123"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	id, err := c.CreateKB(context.Background(), "test-kb")
	if err != nil {
		t.Fatalf("CreateKB: %v", err)
	}
	if id != "kb_abc123" {
		t.Fatalf("kb_id = %q, want kb_abc123", id)
	}
}

func TestUploadDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/knowledgebases/kb_1/documents/file") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing X-API-Key header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_xyz", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.UploadDoc(context.Background(), "kb_1", "test.pdf", strings.NewReader("pdf-data"))
	if err != nil {
		t.Fatalf("UploadDoc: %v", err)
	}
	if doc.ID != "doc_xyz" {
		t.Fatalf("doc_id = %q", doc.ID)
	}
}

func TestUploadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/knowledgebases/kb_1/documents/url") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["url"] != "https://example.com" {
			t.Fatalf("unexpected url %q", body["url"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_url1", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.UploadURL(context.Background(), "kb_1", "https://example.com")
	if err != nil {
		t.Fatalf("UploadURL: %v", err)
	}
	if doc.ID != "doc_url1" {
		t.Fatalf("doc_id = %q", doc.ID)
	}
}

func TestGetDocStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_1", "status": "completed"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.GetDocStatus(context.Background(), "kb_1", "doc_1")
	if err != nil {
		t.Fatalf("GetDocStatus: %v", err)
	}
	if doc.Status != "completed" {
		t.Fatalf("status = %q", doc.Status)
	}
}

func TestDeleteDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	err := c.DeleteDoc(context.Background(), "kb_1", "doc_1")
	if err != nil {
		t.Fatalf("DeleteDoc: %v", err)
	}
}

// Verify that the upload sends file content correctly through the multipart body.
func TestUploadDoc_FileContent(t *testing.T) {
	var gotContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(32 << 20)
		f, _, _ := r.FormFile("file")
		if f != nil {
			data, _ := io.ReadAll(f)
			gotContent = string(data)
			f.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_1", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	c.UploadDoc(context.Background(), "kb_1", "test.txt", strings.NewReader("hello world"))
	if gotContent != "hello world" {
		t.Fatalf("file content = %q, want 'hello world'", gotContent)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/vectoria/... -v
```
Expected: compilation error.

- [ ] **Step 3: Write `client/vectoria/client.go`**

```go
// Package vectoria is the CLI client for the vectoria document/RAG service.
// Auth uses X-API-Key header (not JWT Bearer).
package vectoria

import (
	"net/http"

	"github.com/vibeknow/cli/internal/httpclient"
)

// Client talks to the vectoria service.
type Client struct {
	http *httpclient.Client
}

// New constructs a vectoria client. apiKey is injected as X-API-Key header.
func New(baseURL, apiKey string) *Client {
	chain := httpclient.Chain(http.DefaultTransport,
		apiKeyMiddleware{key: apiKey},
	)
	return &Client{http: httpclient.New(baseURL).WithTransport(chain)}
}

// apiKeyMiddleware injects X-API-Key header on every request.
type apiKeyMiddleware struct{ key string }

func (m apiKeyMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return httpclient.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if m.key != "" {
			r.Header.Set("X-API-Key", m.key)
		}
		return next.RoundTrip(r)
	})
}
```

**Note:** This requires exposing `RoundTripperFunc` from `httpclient`. Add this one-line export to `internal/httpclient/transport.go`:

```go
// RoundTripperFunc is the exported version of roundTripperFunc for use by
// service clients that need custom middleware.
var RoundTripperFunc = func(fn func(*http.Request) (*http.Response, error)) http.RoundTripper {
	return roundTripperFunc(fn)
}
```

- [ ] **Step 4: Write `client/vectoria/knowledgebase.go`**

```go
package vectoria

import (
	"context"
	"fmt"
	"io"
)

// Document represents a vectoria document status.
type Document struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "processing", "completed", "failed"
	Error  string `json:"error,omitempty"`
}

// CreateKB creates a new knowledge base. Returns the kb_id.
func (c *Client) CreateKB(ctx context.Context, name string) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.http.Do(ctx, "POST", "/knowledgebases", map[string]string{"name": name}, &resp); err != nil {
		return "", fmt.Errorf("create knowledgebase: %w", err)
	}
	return resp.ID, nil
}

// UploadDoc uploads a local file to a knowledge base. Returns doc status.
func (c *Client) UploadDoc(ctx context.Context, kbID, fileName string, file io.Reader) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/knowledgebases/%s/documents/file", kbID)
	if err := c.http.DoUpload(ctx, path, "file", fileName, file, &doc); err != nil {
		return nil, fmt.Errorf("upload document: %w", err)
	}
	return &doc, nil
}

// UploadURL submits a URL to a knowledge base for parsing. Returns doc status.
func (c *Client) UploadURL(ctx context.Context, kbID, url string) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/knowledgebases/%s/documents/url", kbID)
	if err := c.http.Do(ctx, "POST", path, map[string]string{"url": url}, &doc); err != nil {
		return nil, fmt.Errorf("upload URL: %w", err)
	}
	return &doc, nil
}

// GetDocStatus polls the status of a document.
func (c *Client) GetDocStatus(ctx context.Context, kbID, docID string) (*Document, error) {
	var doc Document
	path := fmt.Sprintf("/knowledgebases/%s/documents/%s", kbID, docID)
	if err := c.http.Do(ctx, "GET", path, nil, &doc); err != nil {
		return nil, fmt.Errorf("get document status: %w", err)
	}
	return &doc, nil
}

// DeleteDoc deletes a document from a knowledge base.
func (c *Client) DeleteDoc(ctx context.Context, kbID, docID string) error {
	path := fmt.Sprintf("/knowledgebases/%s/documents/%s", kbID, docID)
	if err := c.http.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/vectoria/... -v
```
Expected: all 6 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpclient/transport.go client/vectoria/
git commit -m "feat(vectoria): add vectoria client with KB and document operations"
```

---

## Task 7: figlens client — task, work, export (`client/figlens`)

**Files:**
- Create: `client/figlens/client.go`
- Create: `client/figlens/task.go`
- Create: `client/figlens/work.go`
- Create: `client/figlens/export.go`
- Create: `client/figlens/figlens_test.go`

Standard JWT-auth client. SSE streaming is Task 8 (separate file).

- [ ] **Step 1: Write the failing test**

Create `client/figlens/figlens_test.go`:

```go
package figlens_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

// figlens wraps all responses in {"code":200,"data":{...}}.
func figlensResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data})
}

func TestInitTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tasks/init" || r.Method != "POST" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		figlensResp(w, map[string]any{
			"task_id":    123,
			"session_id": "s_abc",
			"work_id":    "w_xyz",
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	task, err := c.InitTask(context.Background())
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if task.TaskID != 123 {
		t.Fatalf("task_id = %d", task.TaskID)
	}
	if task.SessionID != "s_abc" {
		t.Fatalf("session_id = %q", task.SessionID)
	}
}

func TestGetWorkBySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_id") != "s_abc" {
			t.Fatalf("unexpected session_id query param")
		}
		figlensResp(w, map[string]any{
			"id":         "w_xyz",
			"title":      "Test Video",
			"video_path": "/videos/test.mp4",
			"cover_url":  "https://cover.jpg",
			"duration":   120,
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	work, err := c.GetWorkBySession(context.Background(), "s_abc")
	if err != nil {
		t.Fatalf("GetWorkBySession: %v", err)
	}
	if work.ID != "w_xyz" {
		t.Fatalf("work id = %q", work.ID)
	}
	if work.Duration != 120 {
		t.Fatalf("duration = %d", work.Duration)
	}
}

func TestExportVideo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{"task_id": "export_1"})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	exportID, err := c.ExportVideo(context.Background(), "s_abc")
	if err != nil {
		t.Fatalf("ExportVideo: %v", err)
	}
	if exportID != "export_1" {
		t.Fatalf("export_id = %q", exportID)
	}
}

func TestGetExportResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{
			"status":     "completed",
			"video_path": "/exported/final.mp4",
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	result, err := c.GetExportResult(context.Background(), "export_1")
	if err != nil {
		t.Fatalf("GetExportResult: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestSignedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{"url": "https://signed.example.com/video.mp4"})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	u, err := c.SignedURL(context.Background(), "/videos/test.mp4")
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if u != "https://signed.example.com/video.mp4" {
		t.Fatalf("url = %q", u)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/figlens/... -v
```
Expected: compilation error.

- [ ] **Step 3: Write `client/figlens/client.go`**

```go
// Package figlens is the CLI client for the go-figlens video pipeline service.
package figlens

import (
	"encoding/json"
	"fmt"

	"github.com/vibeknow/cli/internal/httpclient"
)

// Client talks to go-figlens.
type Client struct {
	http *httpclient.Client
}

// New constructs a figlens client with the standard middleware chain.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}

// figlensResponse is the wrapper shape: {"code":200,"data":{...}}.
type figlensResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// doFiglens calls the endpoint and unwraps the {"code","data"} envelope.
func (c *Client) doFiglens(ctx any, method, path string, body, out any) error {
	// We need context.Context but the param is typed as any for the internal helper.
	// This is actually called with a proper context — see usage below.
	return fmt.Errorf("internal error: doFiglens called with wrong type")
}
```

Actually, figlens wraps all responses in `{"code":200,"data":{...}}`. The `httpclient.Client.Do` decodes the *outer* envelope. We need an unwrapping layer. Let me revise — the cleanest approach: figlens client decodes into the wrapper, then re-decodes `data` into the target struct.

Replace `client/figlens/client.go`:

```go
// Package figlens is the CLI client for the go-figlens video pipeline service.
package figlens

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vibeknow/cli/internal/httpclient"
)

// Client talks to go-figlens.
type Client struct {
	http *httpclient.Client
}

// New constructs a figlens client with the standard middleware chain.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}

// envelope is the standard figlens response wrapper: {"code":200,"data":{...}}.
type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// do calls the endpoint, unwraps the figlens envelope, and decodes data into out.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var env envelope
	if err := c.http.Do(ctx, method, path, body, &env); err != nil {
		return err
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("figlens: decode data: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Write `client/figlens/task.go`**

```go
package figlens

import "context"

// Task represents the response from /v1/tasks/init.
type Task struct {
	TaskID    int    `json:"task_id"`
	SessionID string `json:"session_id"`
	WorkID    string `json:"work_id"`
}

// InitTask creates a new task with pipeline mode (v=3).
func (c *Client) InitTask(ctx context.Context) (*Task, error) {
	var t Task
	if err := c.do(ctx, "POST", "/v1/tasks/init", map[string]int{"v": 3}, &t); err != nil {
		return nil, fmt.Errorf("init task: %w", err)
	}
	return &t, nil
}
```

Add missing import to `task.go`:

```go
package figlens

import (
	"context"
	"fmt"
)
```

- [ ] **Step 5: Write `client/figlens/work.go`**

```go
package figlens

import (
	"context"
	"fmt"
)

// Work represents a video work detail.
type Work struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	VideoPath string `json:"video_path"`
	CoverURL  string `json:"cover_url"`
	Duration  int    `json:"duration"` // seconds
}

// GetWorkBySession fetches work detail by session_id.
func (c *Client) GetWorkBySession(ctx context.Context, sessionID string) (*Work, error) {
	var w Work
	path := fmt.Sprintf("/v1/works/detailBySession?session_id=%s", sessionID)
	if err := c.do(ctx, "GET", path, nil, &w); err != nil {
		return nil, fmt.Errorf("get work by session: %w", err)
	}
	return &w, nil
}
```

- [ ] **Step 6: Write `client/figlens/export.go`**

```go
package figlens

import (
	"context"
	"fmt"
)

// ExportResult represents the status of a video export job.
type ExportResult struct {
	Status    string `json:"status"` // "processing", "completed"
	VideoPath string `json:"video_path"`
}

// ExportVideo submits an export job. Returns the export task_id.
func (c *Client) ExportVideo(ctx context.Context, sessionID string) (string, error) {
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/exportRemoteV2",
		map[string]string{"session_id": sessionID}, &resp); err != nil {
		return "", fmt.Errorf("export video: %w", err)
	}
	return resp.TaskID, nil
}

// GetExportResult polls the export job status.
func (c *Client) GetExportResult(ctx context.Context, exportTaskID string) (*ExportResult, error) {
	var r ExportResult
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/exportResultV2",
		map[string]string{"task_id": exportTaskID}, &r); err != nil {
		return nil, fmt.Errorf("get export result: %w", err)
	}
	return &r, nil
}

// SignedURL gets a time-limited signed URL for a video or preview path.
func (c *Client) SignedURL(ctx context.Context, path string) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, "POST", "/v1/agent2forVideo/signedUrl",
		map[string]string{"path": path}, &resp); err != nil {
		return "", fmt.Errorf("signed url: %w", err)
	}
	return resp.URL, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/figlens/... -v
```
Expected: all 5 tests PASS.

- [ ] **Step 8: Commit**

```bash
git add client/figlens/
git commit -m "feat(figlens): add figlens client with task/work/export operations"
```

---

## Task 8: figlens SSE streaming (`client/figlens/stream.go`)

**Files:**
- Create: `client/figlens/stream.go`
- Create: `client/figlens/stream_test.go`

Wraps `internal/sse.Reader` + `internal/stage.FromNode` to parse figlens SSE events into typed CLI events.

- [ ] **Step 1: Write the failing test**

Create `client/figlens/stream_test.go`:

```go
package figlens_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestStreamChat_ProcessEvents(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"start","message":"Starting parse"}}}

data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"success","message":"Parse done"}}}

data: {"code":200,"data":{"type":"aim_result","session_id":"s_abc"}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{
		TaskID:      123,
		SessionID:   "s_abc",
		Query:       "test query",
		KnowledgeID: "kb_1",
		DocID:       "doc_1",
		VoiceID:     "v_1",
	}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected >= 3 events, got %d", len(events))
	}
	// First event: stage.started for "parse"
	if events[0].Type != "stage.started" {
		t.Fatalf("event[0].Type = %q, want stage.started", events[0].Type)
	}
	if events[0].Stage != "parse" {
		t.Fatalf("event[0].Stage = %q, want parse", events[0].Stage)
	}
	// Second event: stage.succeeded for "parse"
	if events[1].Type != "stage.succeeded" {
		t.Fatalf("event[1].Type = %q, want stage.succeeded", events[1].Type)
	}
	// Third event: task.succeeded (from aim_result)
	if events[2].Type != "task.succeeded" {
		t.Fatalf("event[2].Type = %q, want task.succeeded", events[2].Type)
	}
	if events[2].SessionID != "s_abc" {
		t.Fatalf("event[2].SessionID = %q", events[2].SessionID)
	}
}

func TestStreamChat_ErrorEvent(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"error","message":"pipeline failed"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{TaskID: 1, SessionID: "s_1", Query: "test"}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	// StreamChat should return nil — error is delivered as an event, not a Go error.
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed event, got %v", events)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/figlens/ -run TestStream -v
```
Expected: compilation error.

- [ ] **Step 3: Write implementation**

Create `client/figlens/stream.go`:

```go
package figlens

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vibeknow/cli/internal/sse"
	"github.com/vibeknow/cli/internal/stage"
)

// StreamParams are the parameters for the SSE streaming endpoint.
type StreamParams struct {
	TaskID      int    `json:"task_id"`
	SessionID   string `json:"session_id"`
	Query       string `json:"query"`
	KnowledgeID string `json:"knowledge_id,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	VoiceID     string `json:"voice_id,omitempty"`
}

// StreamEvent is a typed event emitted during video generation.
type StreamEvent struct {
	Type      string // "stage.started", "stage.succeeded", "stage.failed", "task.succeeded", "task.failed", "data"
	Stage     string // logical stage name (for stage.* events)
	Node      string // raw figlens node name
	Message   string
	SessionID string // populated on task.succeeded (from aim_result)
}

// ssePayload is the outer figlens SSE data shape: {"code":200,"data":{...}}.
type ssePayload struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// sseData is the inner data object.
type sseData struct {
	Type      string          `json:"type"`
	Log       json.RawMessage `json:"log,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// processLog is the shape of data.log for type=process.
type processLog struct {
	StepID  string `json:"step_id"`
	Status  string `json:"status"` // "start", "success", "error"
	Message string `json:"message"`
}

// StreamChat opens an SSE connection to figlens pipeline mode and calls onEvent
// for each parsed event. Returns when the stream ends ([DONE]) or on error.
func (c *Client) StreamChat(ctx context.Context, params StreamParams, onEvent func(StreamEvent)) error {
	resp, err := c.http.DoRaw(ctx, "POST", "/v1/agent3forVideo/stream", params)
	if err != nil {
		return fmt.Errorf("stream chat: %w", err)
	}
	defer resp.Body.Close()

	reader := sse.NewReader(resp.Body)
	// Track which stages we've already emitted "started" for (dedup: multiple nodes per stage).
	stageStarted := map[string]bool{}

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("sse read: %w", err)
		}

		data := strings.TrimSpace(ev.Data)

		// [DONE] signal
		if data == "[DONE]" {
			return nil
		}

		var payload ssePayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue // skip unparseable lines
		}

		var d sseData
		if err := json.Unmarshal(payload.Data, &d); err != nil {
			continue
		}

		switch d.Type {
		case "process":
			var log processLog
			if err := json.Unmarshal(d.Log, &log); err != nil {
				continue
			}
			stageName, ok := stage.FromNode(log.StepID)
			if !ok {
				continue // unknown node, skip
			}

			switch log.Status {
			case "start":
				if !stageStarted[stageName] {
					stageStarted[stageName] = true
					onEvent(StreamEvent{
						Type:    "stage.started",
						Stage:   stageName,
						Node:    log.StepID,
						Message: log.Message,
					})
				}
			case "success":
				onEvent(StreamEvent{
					Type:    "stage.succeeded",
					Stage:   stageName,
					Node:    log.StepID,
					Message: log.Message,
				})
			case "error":
				onEvent(StreamEvent{
					Type:    "stage.failed",
					Stage:   stageName,
					Node:    log.StepID,
					Message: log.Message,
				})
			}

		case "aim_result":
			sid := d.SessionID
			onEvent(StreamEvent{
				Type:      "task.succeeded",
				SessionID: sid,
			})

		case "error", "ERROR":
			msg := d.Message
			if msg == "" {
				msg = string(payload.Data)
			}
			onEvent(StreamEvent{
				Type:    "task.failed",
				Message: msg,
			})
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/figlens/ -run TestStream -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add client/figlens/stream.go client/figlens/stream_test.go
git commit -m "feat(figlens): add SSE streaming with node-to-stage mapping"
```

---

## Task 9: vibeknow client (`client/vibeknow`)

**Files:**
- Create: `client/vibeknow/client.go`
- Create: `client/vibeknow/voice.go`
- Create: `client/vibeknow/voice_test.go`

Standard JWT client. P2 only needs `ListVoiceTemplates`.

- [ ] **Step 1: Write the failing test**

Create `client/vibeknow/voice_test.go`:

```go
package vibeknow_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/vibeknow"
)

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func TestListVoiceTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/voice-templates" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "1" {
			t.Fatalf("missing page param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"items": []map[string]any{
					{"id": "v_1", "name": "Alice", "language": "en", "gender": "female"},
					{"id": "v_2", "name": "Bob", "language": "zh", "gender": "male"},
				},
			},
		})
	}))
	defer srv.Close()

	c := vibeknow.New(srv.URL, staticToken("tok"))
	voices, err := c.ListVoiceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListVoiceTemplates: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if voices[0].ID != "v_1" || voices[0].Name != "Alice" {
		t.Fatalf("voice[0] = %+v", voices[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/vibeknow/... -v
```
Expected: compilation error.

- [ ] **Step 3: Write `client/vibeknow/client.go`**

```go
// Package vibeknow is the CLI client for the go-vibeknow service
// (billing, voice clone, credits).
package vibeknow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vibeknow/cli/internal/httpclient"
)

// Client talks to go-vibeknow.
type Client struct {
	http *httpclient.Client
}

// New constructs a vibeknow client with the standard middleware chain.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}

// envelope is the standard vibeknow response wrapper: {"code":200,"data":{...}}.
type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// do calls the endpoint, unwraps the envelope, and decodes data into out.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var env envelope
	if err := c.http.Do(ctx, method, path, body, &env); err != nil {
		return err
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("vibeknow: decode data: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Write `client/vibeknow/voice.go`**

```go
package vibeknow

import (
	"context"
	"fmt"
)

// VoiceTemplate represents a built-in voice option.
type VoiceTemplate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Gender     string `json:"gender"`
	PreviewURL string `json:"preview_url,omitempty"`
}

// ListVoiceTemplates fetches all available voice templates.
func (c *Client) ListVoiceTemplates(ctx context.Context) ([]VoiceTemplate, error) {
	var resp struct {
		Items []VoiceTemplate `json:"items"`
	}
	if err := c.do(ctx, "GET", "/v1/voice-templates?page=1&size=100", nil, &resp); err != nil {
		return nil, fmt.Errorf("list voice templates: %w", err)
	}
	return resp.Items, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd ~/laoshen/vibeknow-cli && go test ./client/vibeknow/... -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client/vibeknow/
git commit -m "feat(vibeknow): add vibeknow client with ListVoiceTemplates"
```

---

## Task 10: `doc upload` and `doc get` commands

**Files:**
- Create: `cmd/doc/doc.go`
- Create: `cmd/doc/upload.go`
- Create: `cmd/doc/get.go`

`doc upload <file>` is a shortcut: create KB → upload file → poll until completed → print doc_id. `doc get <id>` returns doc status from vectoria.

- [ ] **Step 1: Write `cmd/doc/doc.go`**

```go
package doc

import "github.com/spf13/cobra"

// Cmd is the `vibeknow doc` parent command.
var Cmd = &cobra.Command{
	Use:   "doc",
	Short: "manage documents in vectoria",
}

func init() {
	Cmd.AddCommand(uploadCmd)
	Cmd.AddCommand(getCmd)
}
```

- [ ] **Step 2: Write `cmd/doc/upload.go`**

```go
package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/cliauth"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "upload a document to vectoria and wait for parsing",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// Validate file exists and is a regular file.
		fi, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", filePath)
		}

		apiKey := os.Getenv("VECTORIA_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("VECTORIA_API_KEY env var is required for vectoria operations")
		}

		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, "vectoria")
		if err != nil {
			return err
		}

		c := vectoria.New(url, apiKey)
		ctx := context.Background()

		// 1. Create KB.
		kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
		kbID, err := c.CreateKB(ctx, kbName)
		if err != nil {
			return fmt.Errorf("create knowledgebase: %w", err)
		}

		// 2. Upload file.
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		doc, err := c.UploadDoc(ctx, kbID, filepath.Base(filePath), f)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}

		// 3. Poll until completed (max 10 minutes, per frontend behavior).
		fmt.Fprintf(os.Stderr, "Parsing document %s...\n", doc.ID)
		deadline := time.Now().Add(10 * time.Minute)
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("document parsing timed out after 10 minutes (doc_id=%s)", doc.ID)
			}
			status, err := c.GetDocStatus(ctx, kbID, doc.ID)
			if err != nil {
				return fmt.Errorf("poll status: %w", err)
			}
			switch status.Status {
			case "completed":
				fmt.Printf("kb_id=%s\ndoc_id=%s\nstatus=completed\n", kbID, doc.ID)
				return nil
			case "failed":
				return fmt.Errorf("document parsing failed: %s", status.Error)
			}
			time.Sleep(2 * time.Second)
		}
	},
}
```

- [ ] **Step 3: Write `cmd/doc/get.go`**

```go
package doc

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

var getFlags struct {
	kbID string
}

var getCmd = &cobra.Command{
	Use:   "get <doc_id>",
	Short: "get a document's status from vectoria",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docID := args[0]
		if getFlags.kbID == "" {
			return fmt.Errorf("--kb-id is required")
		}

		apiKey := os.Getenv("VECTORIA_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("VECTORIA_API_KEY env var is required for vectoria operations")
		}

		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, "vectoria")
		if err != nil {
			return err
		}

		c := vectoria.New(url, apiKey)
		doc, err := c.GetDocStatus(context.Background(), getFlags.kbID, docID)
		if err != nil {
			return err
		}

		fmt.Printf("doc_id=%s\nstatus=%s\n", doc.ID, doc.Status)
		if doc.Error != "" {
			fmt.Printf("error=%s\n", doc.Error)
		}
		return nil
	},
}

func init() {
	getCmd.Flags().StringVar(&getFlags.kbID, "kb-id", "", "knowledge base ID (required)")
	_ = getCmd.MarkFlagRequired("kb-id")
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd ~/laoshen/vibeknow-cli && go build ./cmd/doc/...
```
Expected: compiles without error.

- [ ] **Step 5: Commit**

```bash
git add cmd/doc/
git commit -m "feat(doc): add doc upload and doc get commands"
```

---

## Task 11: `voice list` command

**Files:**
- Create: `cmd/voice/voice.go`
- Create: `cmd/voice/list.go`

- [ ] **Step 1: Write `cmd/voice/voice.go`**

```go
package voice

import "github.com/spf13/cobra"

// Cmd is the `vibeknow voice` parent command.
var Cmd = &cobra.Command{
	Use:   "voice",
	Short: "manage voice templates",
}

func init() {
	Cmd.AddCommand(listCmd)
}
```

- [ ] **Step 2: Write `cmd/voice/list.go`**

```go
package voice

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list available voice templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		tok, _, err := cliauth.ResolverFor(p).Resolve()
		if err != nil {
			return fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
		}
		url, err := endpoints.Resolve(p, "vibeknow")
		if err != nil {
			return err
		}

		c := vibeknow.New(url, staticToken(tok))
		voices, err := c.ListVoiceTemplates(context.Background())
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tLANGUAGE\tGENDER")
		for _, v := range voices {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", v.ID, v.Name, v.Language, v.Gender)
		}
		tw.Flush()
		return nil
	},
}

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }
```

- [ ] **Step 3: Verify compilation**

```bash
cd ~/laoshen/vibeknow-cli && go build ./cmd/voice/...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/voice/
git commit -m "feat(voice): add voice list command"
```

---

## Task 12: `video status/wait/download` commands

**Files:**
- Create: `cmd/video/video.go`
- Create: `cmd/video/status.go`
- Create: `cmd/video/wait.go`
- Create: `cmd/video/download.go`

`status` and `wait` both reconnect to the SSE stream by POSTing with an empty query and existing task_id+session_id. `download` handles the export→poll→signedUrl→download pipeline.

- [ ] **Step 1: Write `cmd/video/video.go`**

```go
package video

import "github.com/spf13/cobra"

// Cmd is the `vibeknow video` parent command.
var Cmd = &cobra.Command{
	Use:   "video",
	Short: "manage video tasks",
}

func init() {
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(waitCmd)
	Cmd.AddCommand(downloadCmd)
}
```

- [ ] **Step 2: Write shared helpers at `cmd/video/helpers.go`**

Common logic for building figlens clients from the current profile:

```go
package video

import (
	"context"
	"fmt"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func newFiglensClient() (*figlens.Client, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	tok, _, err := cliauth.ResolverFor(p).Resolve()
	if err != nil {
		return nil, fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
	}
	url, err := endpoints.Resolve(p, "figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, staticToken(tok)), nil
}
```

- [ ] **Step 3: Write `cmd/video/status.go`**

```go
package video

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var statusFlags struct {
	sessionID string
}

var statusCmd = &cobra.Command{
	Use:   "status <task_id>",
	Short: "show the current status of a video task",
	Long:  "Reconnects to the figlens SSE stream to retrieve the latest task state.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if statusFlags.sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		work, err := c.GetWorkBySession(context.Background(), statusFlags.sessionID)
		if err != nil {
			return fmt.Errorf("get work: %w", err)
		}

		fmt.Printf("session_id=%s\n", statusFlags.sessionID)
		fmt.Printf("work_id=%s\n", work.ID)
		fmt.Printf("title=%s\n", work.Title)
		fmt.Printf("duration=%ds\n", work.Duration)
		if work.VideoPath != "" {
			fmt.Printf("video_path=%s\n", work.VideoPath)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&statusFlags.sessionID, "session-id", "", "session ID (required)")
	_ = statusCmd.MarkFlagRequired("session-id")
}
```

- [ ] **Step 4: Write `cmd/video/wait.go`**

```go
package video

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
)

var waitFlags struct {
	sessionID string
}

var waitCmd = &cobra.Command{
	Use:   "wait <task_id>",
	Short: "wait for a video task to complete (reconnects to SSE stream)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("task_id must be an integer: %w", err)
		}
		if waitFlags.sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}

		params := figlens.StreamParams{
			TaskID:    taskID,
			SessionID: waitFlags.sessionID,
			Query:     "", // empty query = reconnect/resume
		}

		fmt.Fprintln(os.Stderr, "Waiting for task to complete...")
		var lastEvent figlens.StreamEvent

		err = c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
			lastEvent = ev
			switch ev.Type {
			case "stage.started":
				fmt.Fprintf(os.Stderr, "  [%s] started\n", ev.Stage)
			case "stage.succeeded":
				fmt.Fprintf(os.Stderr, "  [%s] done\n", ev.Stage)
			case "stage.failed":
				fmt.Fprintf(os.Stderr, "  [%s] FAILED: %s\n", ev.Stage, ev.Message)
			case "task.succeeded":
				fmt.Fprintf(os.Stderr, "Task completed.\n")
			case "task.failed":
				fmt.Fprintf(os.Stderr, "Task failed: %s\n", ev.Message)
			}
		})
		if err != nil {
			return fmt.Errorf("stream interrupted: %w", err)
		}

		if lastEvent.Type == "task.succeeded" && lastEvent.SessionID != "" {
			work, err := c.GetWorkBySession(context.Background(), lastEvent.SessionID)
			if err == nil {
				fmt.Printf("session_id=%s\nwork_id=%s\ntitle=%s\n", lastEvent.SessionID, work.ID, work.Title)
			}
		}
		if lastEvent.Type == "task.failed" {
			os.Exit(5) // exit code 5 = task failed, not retryable
		}
		return nil
	},
}

func init() {
	waitCmd.Flags().StringVar(&waitFlags.sessionID, "session-id", "", "session ID (required)")
	_ = waitCmd.MarkFlagRequired("session-id")
}
```

- [ ] **Step 5: Write `cmd/video/download.go`**

```go
package video

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var downloadFlags struct {
	sessionID string
	output    string
	overwrite bool
}

var downloadCmd = &cobra.Command{
	Use:   "download <task_id>",
	Short: "export and download a completed video",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if downloadFlags.sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}

		c, err := newFiglensClient()
		if err != nil {
			return err
		}
		ctx := context.Background()

		// 1. Check if video_path already exists (previously exported).
		work, err := c.GetWorkBySession(ctx, downloadFlags.sessionID)
		if err != nil {
			return fmt.Errorf("get work: %w", err)
		}

		videoPath := work.VideoPath
		if videoPath == "" {
			// 2. Submit export job.
			fmt.Fprintln(os.Stderr, "Exporting video...")
			exportID, err := c.ExportVideo(ctx, downloadFlags.sessionID)
			if err != nil {
				return fmt.Errorf("submit export: %w", err)
			}

			// 3. Poll export status (every 3s, max 10 minutes).
			deadline := time.Now().Add(10 * time.Minute)
			for {
				if time.Now().After(deadline) {
					return fmt.Errorf("export timed out after 10 minutes")
				}
				result, err := c.GetExportResult(ctx, exportID)
				if err != nil {
					return fmt.Errorf("poll export: %w", err)
				}
				if result.Status == "completed" {
					videoPath = result.VideoPath
					break
				}
				time.Sleep(3 * time.Second)
			}
		}

		// 4. Get signed URL.
		signedURL, err := c.SignedURL(ctx, videoPath)
		if err != nil {
			return fmt.Errorf("get signed url: %w", err)
		}

		// 5. Download file.
		outPath := downloadFlags.output
		if outPath == "" {
			outPath = args[0] + ".mp4"
		}

		if !downloadFlags.overwrite {
			if _, err := os.Stat(outPath); err == nil {
				return fmt.Errorf("file %s already exists (use --overwrite to replace)", outPath)
			}
		}

		fmt.Fprintf(os.Stderr, "Downloading to %s...\n", outPath)
		resp, err := http.Get(signedURL)
		if err != nil {
			return fmt.Errorf("download: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
		}

		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()

		written, err := io.Copy(f, resp.Body)
		if err != nil {
			os.Remove(outPath) // clean up partial download
			return fmt.Errorf("download interrupted: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Done (%d bytes)\n", written)
		fmt.Println(outPath)
		return nil
	},
}

func init() {
	downloadCmd.Flags().StringVar(&downloadFlags.sessionID, "session-id", "", "session ID (required)")
	downloadCmd.Flags().StringVar(&downloadFlags.output, "output", "", "output file path (default: <task_id>.mp4)")
	downloadCmd.Flags().BoolVar(&downloadFlags.overwrite, "overwrite", false, "overwrite existing file")
	_ = downloadCmd.MarkFlagRequired("session-id")
}
```

- [ ] **Step 6: Verify compilation**

```bash
cd ~/laoshen/vibeknow-cli && go build ./cmd/video/...
```

- [ ] **Step 7: Commit**

```bash
git add cmd/video/
git commit -m "feat(video): add video status, wait, and download commands"
```

---

## Task 13: Hero command — `vibeknow create`

**Files:**
- Create: `cmd/create.go`

The crown jewel. Chains: `--from` parsing → vectoria KB+doc → figlens task init → SSE stream → work detail. Supports `--async` and `--voice`.

- [ ] **Step 1: Write `cmd/create.go`**

```go
package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

var createFlags struct {
	from    string
	voice   string
	async   bool
}

var docIDRe = regexp.MustCompile(`^doc_[a-zA-Z0-9]{8,}$`)

type staticTokenForCreate string

func (s staticTokenForCreate) Token(ctx context.Context) (string, error) { return string(s), nil }

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "create a video from a document or URL (hero command)",
	Long: `Turn a local file, URL, or existing doc_id into a video.

Examples:
  vibeknow create --from report.pdf
  vibeknow create --from https://example.com/article
  vibeknow create --from doc_abc123xyz --voice v_female_01
  vibeknow create --from report.pdf --async`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createFlags.from == "" {
			return fmt.Errorf("--from is required")
		}

		// Resolve credential and endpoints.
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		tok, _, err := cliauth.ResolverFor(p).Resolve()
		if err != nil {
			return fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
		}
		figlensURL, err := endpoints.Resolve(p, "figlens")
		if err != nil {
			return err
		}
		vectoriaURL, err := endpoints.Resolve(p, "vectoria")
		if err != nil {
			return err
		}
		vectoriaKey := os.Getenv("VECTORIA_API_KEY")
		if vectoriaKey == "" {
			return fmt.Errorf("VECTORIA_API_KEY env var is required")
		}

		fc := figlens.New(figlensURL, staticTokenForCreate(tok))
		vc := vectoria.New(vectoriaURL, vectoriaKey)
		ctx := context.Background()

		// --- Phase 1: Resolve --from to kb_id + doc_id ---
		var kbID, docID string

		source := createFlags.from
		switch {
		case docIDRe.MatchString(source):
			// Priority 1: literal doc_id — skip upload.
			// User must have already uploaded; we can't derive kb_id.
			// For now, we pass empty kb_id — figlens only needs doc_id.
			docID = source

		case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
			// Priority 3: URL.
			if _, err := url.ParseRequestURI(source); err != nil {
				return fmt.Errorf("invalid URL: %s", source)
			}
			kbID, docID, err = uploadURLAndWait(ctx, vc, source)
			if err != nil {
				return err
			}

		default:
			// Priority 2+4: local file path.
			absPath, err := filepath.Abs(source)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			fi, err := os.Stat(absPath)
			if err != nil {
				return fmt.Errorf("file not found: %s", source)
			}
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("not a regular file: %s", source)
			}
			kbID, docID, err = uploadFileAndWait(ctx, vc, absPath)
			if err != nil {
				return err
			}
		}

		// --- Phase 2: Init figlens task ---
		task, err := fc.InitTask(ctx)
		if err != nil {
			return fmt.Errorf("init task: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Task created: %d (session=%s)\n", task.TaskID, task.SessionID)

		// --async: print task info and exit immediately.
		if createFlags.async {
			fmt.Printf("task_id=%d\nsession_id=%s\n", task.TaskID, task.SessionID)
			fmt.Fprintf(os.Stderr, "Use `vibeknow video wait %d --session-id %s` to follow progress.\n",
				task.TaskID, task.SessionID)
			return nil
		}

		// --- Phase 3: Stream SSE ---
		params := figlens.StreamParams{
			TaskID:      task.TaskID,
			SessionID:   task.SessionID,
			Query:       "generate video", // default query for pipeline mode
			KnowledgeID: kbID,
			DocID:       docID,
			VoiceID:     createFlags.voice,
		}

		var resultSessionID string
		var taskFailed bool
		var failMessage string

		err = fc.StreamChat(ctx, params, func(ev figlens.StreamEvent) {
			switch ev.Type {
			case "stage.started":
				fmt.Fprintf(os.Stderr, "  [%s] started...\n", ev.Stage)
			case "stage.succeeded":
				fmt.Fprintf(os.Stderr, "  [%s] done\n", ev.Stage)
			case "stage.failed":
				fmt.Fprintf(os.Stderr, "  [%s] FAILED: %s\n", ev.Stage, ev.Message)
			case "task.succeeded":
				resultSessionID = ev.SessionID
				fmt.Fprintln(os.Stderr, "Video generation complete!")
			case "task.failed":
				taskFailed = true
				failMessage = ev.Message
				fmt.Fprintf(os.Stderr, "Task failed: %s\n", ev.Message)
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Stream interrupted: %v\n", err)
			fmt.Fprintf(os.Stderr, "Run `vibeknow video wait %d --session-id %s` to resume.\n",
				task.TaskID, task.SessionID)
			os.Exit(6) // exit code 6: stream interrupted
		}

		if taskFailed {
			fmt.Fprintf(os.Stderr, "Error: %s\n", failMessage)
			os.Exit(5) // exit code 5: task failed
		}

		// --- Phase 4: Fetch work detail ---
		if resultSessionID != "" {
			work, err := fc.GetWorkBySession(ctx, resultSessionID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not fetch work detail: %v\n", err)
			} else {
				fmt.Printf("task_id=%s\nsession_id=%s\nwork_id=%s\ntitle=%s\nduration=%ds\n",
					strconv.Itoa(task.TaskID), resultSessionID, work.ID, work.Title, work.Duration)
				fmt.Fprintf(os.Stderr, "Download with: vibeknow video download %d --session-id %s\n",
					task.TaskID, resultSessionID)
			}
		}

		return nil
	},
}

func uploadFileAndWait(ctx context.Context, vc *vectoria.Client, filePath string) (kbID, docID string, err error) {
	fmt.Fprintln(os.Stderr, "Creating knowledge base...")
	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	kbID, err = vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", fmt.Errorf("create KB: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Uploading %s...\n", filepath.Base(filePath))
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	doc, err := vc.UploadDoc(ctx, kbID, filepath.Base(filePath), f)
	if err != nil {
		return "", "", fmt.Errorf("upload: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Waiting for document parsing...")
	if err := waitForDoc(ctx, vc, kbID, doc.ID); err != nil {
		return "", "", err
	}
	return kbID, doc.ID, nil
}

func uploadURLAndWait(ctx context.Context, vc *vectoria.Client, rawURL string) (kbID, docID string, err error) {
	fmt.Fprintln(os.Stderr, "Creating knowledge base...")
	kbName := fmt.Sprintf("vibeknow-cli-%d", time.Now().Unix())
	kbID, err = vc.CreateKB(ctx, kbName)
	if err != nil {
		return "", "", fmt.Errorf("create KB: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Submitting URL: %s\n", rawURL)
	doc, err := vc.UploadURL(ctx, kbID, rawURL)
	if err != nil {
		return "", "", fmt.Errorf("upload URL: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Waiting for document parsing...")
	if err := waitForDoc(ctx, vc, kbID, doc.ID); err != nil {
		return "", "", err
	}
	return kbID, doc.ID, nil
}

func waitForDoc(ctx context.Context, vc *vectoria.Client, kbID, docID string) error {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("document parsing timed out (10 min)")
		}
		status, err := vc.GetDocStatus(ctx, kbID, docID)
		if err != nil {
			return fmt.Errorf("poll: %w", err)
		}
		switch status.Status {
		case "completed":
			fmt.Fprintln(os.Stderr, "Document parsed.")
			return nil
		case "failed":
			return fmt.Errorf("document parsing failed: %s", status.Error)
		}
		time.Sleep(2 * time.Second)
	}
}

func init() {
	createCmd.Flags().StringVar(&createFlags.from, "from", "", "source: local file path, URL, or doc_id (required)")
	createCmd.Flags().StringVar(&createFlags.voice, "voice", "", "voice template ID")
	createCmd.Flags().BoolVar(&createFlags.async, "async", false, "submit task and exit immediately")
	_ = createCmd.MarkFlagRequired("from")
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd ~/laoshen/vibeknow-cli && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/create.go
git commit -m "feat: add hero create command — file/URL to video in one command"
```

---

## Task 14: Register new commands in root

**Files:**
- Modify: `cmd/root.go`

Wire all new commands into the root command tree.

- [ ] **Step 1: Update `cmd/root.go` imports and init**

Add imports and `rootCmd.AddCommand` calls:

```go
import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	apicmd "github.com/vibeknow/cli/cmd/api"
	authcmd "github.com/vibeknow/cli/cmd/auth"
	configcmd "github.com/vibeknow/cli/cmd/config"
	doccmd "github.com/vibeknow/cli/cmd/doc"
	profilecmd "github.com/vibeknow/cli/cmd/profile"
	videocmd "github.com/vibeknow/cli/cmd/video"
	voicecmd "github.com/vibeknow/cli/cmd/voice"
	"github.com/vibeknow/cli/internal/i18n"
)
```

Add to `init()`:

```go
rootCmd.AddCommand(createCmd)
rootCmd.AddCommand(doccmd.Cmd)
rootCmd.AddCommand(videocmd.Cmd)
rootCmd.AddCommand(voicecmd.Cmd)
```

- [ ] **Step 2: Verify full build**

```bash
cd ~/laoshen/vibeknow-cli && go build -o vibeknow . && ./vibeknow --help
```

Expected: help output shows `create`, `doc`, `video`, `voice` alongside existing commands.

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "feat: register create/doc/video/voice commands in root"
```

---

## Task 15: Integration test — create flow end-to-end

**Files:**
- Create: `tests/integration/create_flow_test.go`

httptest-based integration test that fakes vectoria + figlens and runs the full create pipeline.

- [ ] **Step 1: Write integration test**

Create `tests/integration/create_flow_test.go`:

```go
package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateFlow_FileToVideo exercises the full hero create command end-to-end
// with httptest fakes for vectoria and figlens.
func TestCreateFlow_FileToVideo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	// Fake vectoria: KB create + doc upload + doc status poll.
	vectoria := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/knowledgebases":
			json.NewEncoder(w).Encode(map[string]string{"id": "kb_test"})

		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/documents/file"):
			// Consume multipart to avoid broken pipe.
			r.ParseMultipartForm(32 << 20)
			json.NewEncoder(w).Encode(map[string]string{"id": "doc_test", "status": "completed"})

		case r.Method == "GET" && strings.Contains(r.URL.Path, "/documents/"):
			json.NewEncoder(w).Encode(map[string]string{"id": "doc_test", "status": "completed"})

		default:
			w.WriteHeader(404)
		}
	}))
	defer vectoria.Close()

	// Fake figlens: task init + SSE stream + work detail.
	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/tasks/init":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"task_id": 1, "session_id": "s_test", "work_id": "w_test",
				},
			})

		case r.URL.Path == "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			flusher, _ := w.(http.Flusher)
			events := []string{
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"start","message":"go"}}}`,
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"success","message":"ok"}}}`,
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_test"}}`,
				`data: [DONE]`,
			}
			for _, e := range events {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w) // blank line = event boundary
				if flusher != nil {
					flusher.Flush()
				}
			}

		case r.URL.Path == "/v1/works/detailBySession":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"id": "w_test", "title": "Test Video", "video_path": "/test.mp4",
					"cover_url": "", "duration": 30,
				},
			})

		default:
			w.WriteHeader(404)
			io.WriteString(w, "not found: "+r.URL.Path)
		}
	}))
	defer figlens.Close()

	// Create a temp file to upload.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content for video generation"), 0644)

	// Create a temp profile pointing at our test servers.
	profileDir := filepath.Join(tmpDir, "config")
	os.MkdirAll(profileDir, 0755)
	profileYAML := fmt.Sprintf(`schema_version: "2"
current: test
profiles:
  - name: test
    endpoints:
      vectoria: %s
      figlens: %s
    credential_ref: test
    trust: dev
    is_production: false
`, vectoria.URL, figlens.URL)
	os.WriteFile(filepath.Join(profileDir, "profiles.yaml"), []byte(profileYAML), 0644)

	// Build the binary.
	binary := filepath.Join(tmpDir, "vibeknow")
	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	buildCmd.Dir = os.Getenv("HOME") + "/laoshen/vibeknow-cli"
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Run the create command.
	cmd := exec.Command(binary, "create", "--from", testFile)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VECTORIA_API_KEY=fake-key",
		"XDG_CONFIG_HOME="+profileDir, // override config dir
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create command failed: %v\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "task_id=1") {
		t.Errorf("expected task_id=1 in output:\n%s", output)
	}
	if !strings.Contains(output, "work_id=w_test") {
		t.Errorf("expected work_id=w_test in output:\n%s", output)
	}
}
```

- [ ] **Step 2: Run integration test**

```bash
cd ~/laoshen/vibeknow-cli && go test ./tests/integration/ -run TestCreateFlow -v -count=1
```

Expected: PASS (may need adjustment based on config path resolution — fix as needed).

- [ ] **Step 3: Commit**

```bash
git add tests/integration/create_flow_test.go
git commit -m "test: add end-to-end integration test for create flow"
```

---

## Self-review notes

**Spec coverage check:**
- Hero `create --from` with file/URL/doc_id parsing: Task 13 (spec §6.1) ✅
- `--async` mode: Task 13 ✅
- SSE streaming from figlens: Tasks 2, 8 (spec §5.3) ✅
- Node→stage mapping (14→6): Task 3 (spec §5.3) ✅
- `doc upload/get`: Task 10 ✅
- `voice list`: Task 11 ✅
- `video status/wait/download`: Task 12 (spec §5.2) ✅
- vectoria X-API-Key auth: Task 6 ✅
- figlens envelope unwrap: Task 7 ✅
- Exit codes 5/6: Tasks 12, 13 ✅
- `--output json/ndjson` for create: NOT in P2 scope — TTY text output only in initial delivery. JSON/NDJSON output mode for `create` is an enhancement that can be added by wrapping the event callback with NDJSON emission. Tracked as follow-up.
- `project` commands: explicitly deferred (backend doesn't exist).
- `rag query`: explicitly deferred (separate plan).

**Type consistency check:**
- `httpclient.Client` / `httpclient.TokenProvider` / `httpclient.StandardChain` — used consistently across all 3 new service clients.
- `figlens.StreamEvent` / `figlens.StreamParams` — consistent between stream.go and cmd/video/wait.go, cmd/create.go.
- `vectoria.Document` — used in both knowledgebase.go and cmd/doc/upload.go.
- `staticToken` type — defined locally in each cmd package (voice, video, create) following the P1 pattern from cmd/auth/whoami.go. Not ideal but matches existing convention.

**Placeholder scan:** No TBD/TODO/placeholders found.
