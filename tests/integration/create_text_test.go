package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

type uploadedDoc struct {
	mu       sync.Mutex
	fileName string
	body     string
	kbCount  int
}

func (u *uploadedDoc) snapshot() (string, string, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.fileName, u.body, u.kbCount
}

// textStack mocks the two services a paste travels through: vectoria takes
// the document, figlens starts the run.
func textStack(t *testing.T, up *uploadedDoc) (figlensURL, vectoriaURL string, closeAll func()) {
	t.Helper()

	// vectoria answers with flat JSON — it does not use figlens' code/data
	// envelope.
	vectoria := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/knowledgebases" && r.Method == "POST":
			up.mu.Lock()
			up.kbCount++
			up.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "kb_text"})

		case strings.HasSuffix(r.URL.Path, "/documents/file"):
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				w.WriteHeader(400)
				return
			}
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				if part.FormName() == "file" {
					b, _ := io.ReadAll(part)
					up.mu.Lock()
					up.fileName = part.FileName()
					up.body = string(b)
					up.mu.Unlock()
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "doc_text", "status": "completed"})

		default:
			// Document status polling and anything else: report ready.
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "doc_text", "status": "completed"})
		}
	}))

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 7, "session_id": "s_text", "work_id": 8, "v": 3},
			})

		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintln(w, `data: {"code":200,"data":{"type":"aim_result","answer_done":{"text":"prompt"}}}`)
			fmt.Fprintln(w)
			fmt.Fprintln(w, `data: [DONE]`)
			fmt.Fprintln(w)

		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintln(w, `data: {"code":200,"data":{"type":"process","log":{"step_id":"big_director","status":"start","message":"go"}}}`)
			fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}

		default:
			w.WriteHeader(404)
		}
	}))

	return figlens.URL, vectoria.URL, func() {
		figlens.Close()
		vectoria.Close()
	}
}

func TestCreate_Text_UploadsThePasteAsADocument(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	up := &uploadedDoc{}
	figlensURL, vectoriaURL, closeAll := textStack(t, up)
	defer closeAll()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"figlens": figlensURL, "vectoria": vectoriaURL})

	const pasted = "季度复盘要点\n\n第一，收入同比增长。\n第二，成本下降。"
	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--text", pasted, "--async", "--output", "json",
	)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	name, body, kbs := up.snapshot()

	// The text arrives byte for byte. Anything else means the paste was
	// reshaped on the way through, which is the failure the flag exists to
	// remove — the shell was doing it before.
	if body != pasted {
		t.Errorf("uploaded body = %q, want the text unchanged", body)
	}
	// Named after its opening line, so several pastes in one session are
	// distinguishable in `vk doc` and the knowledge base.
	if name != "季度复盘要点.md" {
		t.Errorf("uploaded file name = %q, want it derived from the first line", name)
	}
	if kbs != 1 {
		t.Errorf("created %d knowledge bases, want exactly 1", kbs)
	}
}

func TestCreate_FromStdin_ReadsThePaste(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	up := &uploadedDoc{}
	figlensURL, vectoriaURL, closeAll := textStack(t, up)
	defer closeAll()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"figlens": figlensURL, "vectoria": vectoriaURL})

	// A heredoc is how an agent gets multi-line text in without the shell
	// touching it, so stdin is the path that matters most for long pastes.
	const pasted = "标题行\n带 $VAR、`反引号` 和 \"引号\" 的正文"

	cmd := exec.Command(bin, "create", "--from", "-", "--async", "--output", "json")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
	)
	cmd.Stdin = strings.NewReader(pasted)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("create --from - failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	_, body, _ := up.snapshot()
	if body != pasted {
		t.Errorf("uploaded body = %q, want the text unchanged", body)
	}
}

func TestCreate_TextAndFrom_AreRefusedTogether(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	up := &uploadedDoc{}
	figlensURL, vectoriaURL, closeAll := textStack(t, up)
	defer closeAll()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"figlens": figlensURL, "vectoria": vectoriaURL})

	// Both name the source. Picking one and dropping the other would leave
	// the caller with a video built from an input it did not choose, and no
	// way to find out.
	_, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--text", "some text", "--from", "./report.pdf", "--async",
	)
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr: %s", code, stderr)
	}

	if _, _, kbs := up.snapshot(); kbs != 0 {
		t.Errorf("refused before any upload, but %d knowledge bases were created", kbs)
	}
}

func TestCreate_NeitherTextNorFrom_NamesBoth(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	up := &uploadedDoc{}
	figlensURL, vectoriaURL, closeAll := textStack(t, up)
	defer closeAll()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"figlens": figlensURL, "vectoria": vectoriaURL})

	_, stderr, code := runVideoCmd(t, bin, configHome, "create", "--async")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr: %s", code, stderr)
	}
	// The message has to name the new way in as well: a caller holding text
	// and reading "--from is required" goes looking for a file to write.
	if !strings.Contains(stderr, "--text") {
		t.Errorf("the error should mention --text, got: %s", stderr)
	}
}
