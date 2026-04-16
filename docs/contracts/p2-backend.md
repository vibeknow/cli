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
