package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// slowStream serves a pipeline that reports one stage and then goes quiet,
// which is the shape of every real run's long middle: the caller has to be
// able to say where it got to without the run being finished.
func slowStream(t *testing.T, hold time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, e := range []string{
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"big_director","status":"start","message":"planning"}}}`,
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"tts_generate","status":"start","message":"voicing"}}}`,
			} {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
			// No terminal event: the run is still going when the budget ends.
			select {
			case <-r.Context().Done():
			case <-time.After(hold):
			}

		default:
			w.WriteHeader(404)
		}
	}))
}

// errEnvelope pulls the JSON error envelope out of stderr.
//
// Under --output json, stderr carries the live `vk_event=` progress lines as
// well as the envelope — deliberately, so one call gives both a parsable
// result and a running commentary. The envelope is what remains once those
// are dropped.
func errEnvelope(t *testing.T, stderr string) string {
	t.Helper()
	var kept []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "vk_event=") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestVideoWait_ForBudget_ReportsProgressAndKeepsTheRun(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := slowStream(t, 30*time.Second)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	start := time.Now()
	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "wait", "42", "--session-id", "s_budget", "--for", "2s", "--output", "json",
	)
	elapsed := time.Since(start)

	// Exit 6, not 0: `wait` exiting 0 means the task succeeded. A caller
	// running `wait && download` must not be sent after a video that is
	// still being made.
	if code != 6 {
		t.Fatalf("exit %d, want 6\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// The point of the flag is that it comes back on time. A budget that
	// overruns is a budget the host's own timeout will interrupt instead,
	// which is the failure it exists to avoid.
	if elapsed > 20*time.Second {
		t.Fatalf("waited %s for a 2s budget", elapsed)
	}

	// The JSON error envelope is the only form a scripted caller reads.
	var env struct {
		Error struct {
			Code   int `json:"code"`
			Detail struct {
				Status      string `json:"status"`
				Reason      string `json:"reason"`
				Stage       string `json:"stage"`
				SessionID   string `json:"session_id"`
				WaitedMs    int64  `json:"waited_ms"`
				NextActions []struct {
					Command string `json:"command"`
				} `json:"next_actions"`
			} `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errEnvelope(t, stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\nstderr: %s", err, stderr)
	}

	d := env.Error.Detail
	if d.Reason != "wait_budget_expired" {
		t.Errorf("reason = %q, want wait_budget_expired", d.Reason)
	}
	if d.Status != "running" {
		t.Errorf("status = %q, want running", d.Status)
	}
	// The last stage actually seen, not the first: a caller repeating this
	// every couple of minutes should watch the answer move.
	if d.Stage != "tts / tts_generate" {
		t.Errorf("stage = %q, want the last stage on the wire", d.Stage)
	}
	if d.SessionID != "s_budget" {
		t.Errorf("session_id = %q, want s_budget", d.SessionID)
	}
	if d.WaitedMs <= 0 {
		t.Errorf("waited_ms = %d, want the time actually spent", d.WaitedMs)
	}
	if len(d.NextActions) != 1 || !strings.Contains(d.NextActions[0].Command, "--for") {
		t.Errorf("next_actions should hand back a runnable wait, got %#v", d.NextActions)
	}

	// Nothing on stdout: the run has produced no result yet, and printing a
	// snapshot of one would be a second, contradicting answer.
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty while the run is unfinished, got: %s", stdout)
	}
}

func TestVideoWait_ForRejectsANonPositiveBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := slowStream(t, time.Second)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	for _, arg := range []string{"0s", "-5s"} {
		t.Run(arg, func(t *testing.T) {
			// Refused rather than treated as "no budget": a zero budget
			// silently becoming an unbounded wait is the exact hang the flag
			// was added to prevent.
			_, stderr, code := runVideoCmd(t, bin, configHome,
				"video", "wait", "42", "--session-id", "s_x", "--for", arg,
			)
			if code != 2 {
				t.Fatalf("exit %d, want 2 for --for %s\nstderr: %s", code, arg, stderr)
			}
		})
	}
}

// Without --for, wait keeps its old contract: it does not come back early,
// and a stream that ends with no terminal event is an unknown state rather
// than a run that is merely still going.
func TestVideoWait_WithoutFor_KeepsTheUnknownStateContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintln(w, `data: {"code":200,"data":{"type":"process","log":{"step_id":"big_director","status":"start","message":"planning"}}}`)
			fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}
			// Closes without a terminal event.

		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"id": 43, "session_id": "s_plain", "status": 1},
			})

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "wait", "42", "--session-id", "s_plain", "--output", "json",
	)
	if code != 6 {
		t.Fatalf("exit %d, want 6\nstderr: %s", code, stderr)
	}
	if strings.Contains(stderr, "wait_budget_expired") {
		t.Errorf("a run with no --for must not report a spent budget: %s", stderr)
	}
}
