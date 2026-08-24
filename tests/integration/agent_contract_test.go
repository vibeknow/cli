package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUsageErrorsExitTwo pins the exit code for a malformed command line.
//
// These are the mistakes a caller makes most often — a flag remembered from a
// sibling command, a typo, a forgotten required flag — and they are the most
// trivially self-correctable. They used to exit 1, which in this CLI means
// "generic failure, read stderr", so a caller branching on exit codes could
// not tell "your arguments are wrong, fix them" from "the backend broke".
//
// Cobra reports them as plain errors matched by message prefix in
// cmd/exitcode.go, so this test doubles as the guard for a cobra upgrade that
// rewords them: it drives each through the real command tree.
func TestUsageErrorsExitTwo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildVideoProfile(t, "http://127.0.0.1:1")

	cases := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"bogus"}},
		{"unknown subcommand", []string{"video", "bogus"}},
		{"unknown flag", []string{"version", "--bogus"}},
		{"flag needs an argument", []string{"create", "--from"}},
		{"missing required flag", []string{"api", "call", "--service", "figlens"}},
		{"too many args", []string{"version", "extra", "args", "here"}},
		{"missing required value", []string{"create"}},
		{"bad enum value", []string{"create", "--from", "x", "--mode", "nope"}},
		{"bad output format", []string{"version", "--output", "yaml"}},
		{"unknown api service", []string{"api", "call", "--service", "nope", "--path", "/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runVideoCmd(t, bin, configHome, tc.args...)
			if code != 2 {
				t.Fatalf("a malformed command line must exit 2, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("nothing should reach stdout for a rejected command: %q", stdout)
			}
		})
	}
}

// TestBareGroupStillPrintsHelp guards the other half of the NoArgs change.
// Group commands had to be made Runnable for arg validation to run at all
// (cobra short-circuits a non-runnable command straight to help, skipping
// validation) — so this checks that making them runnable did not cost the
// plain `vk video` behaviour a person relies on.
func TestBareGroupStillPrintsHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildVideoProfile(t, "http://127.0.0.1:1")

	for _, group := range []string{"video", "jobs", "auth", "kb", "doc", "config", "profile", "credits", "voice", "api"} {
		t.Run(group, func(t *testing.T) {
			stdout, stderr, code := runVideoCmd(t, bin, configHome, group)
			if code != 0 {
				t.Fatalf("`vk %s` should print help and exit 0, got %d\nstderr: %s", group, code, stderr)
			}
			if !strings.Contains(stdout, "Available Commands") {
				t.Fatalf("`vk %s` should list its subcommands, got: %s", group, stdout)
			}
			// --help must keep working too.
			h, _, hc := runVideoCmd(t, bin, configHome, group, "--help")
			if hc != 0 || !strings.Contains(h, "Available Commands") {
				t.Fatalf("`vk %s --help` broke: exit %d\n%s", group, hc, h)
			}
		})
	}
}

// TestUnknownFlagSuggestsNearest: naming the intended flag turns a
// read-the-docs round trip into a one-token correction. The cases are real
// cross-command confusions and typos, not synthetic ones.
func TestUnknownFlagSuggestsNearest(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildVideoProfile(t, "http://127.0.0.1:1")

	cases := []struct{ typed, want string }{
		{"--kbid", "--kb-id"},
		{"--outputs", "--output"},
		{"--voice-id", "--voice"},
	}
	for _, tc := range cases {
		t.Run(tc.typed, func(t *testing.T) {
			_, stderr, code := runVideoCmd(t, bin, configHome, "create", "--from", "x", tc.typed, "v")
			if code != 2 {
				t.Fatalf("exit %d, want 2: %s", code, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("the hint should name %s, got: %s", tc.want, stderr)
			}
		})
	}
}

// TestFlagAliases: the CLI grew two words for the same idea (--size vs
// --limit for a row cap, --no-wait vs --async for "do not block"). Rather
// than make a caller remember which command uses which, the alternate
// spelling is normalized at parse time. --help still shows one name each.
func TestFlagAliases(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := agentStub(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	// --size is video/kb's spelling; jobs list says --limit.
	if _, stderr, code := runVideoCmd(t, bin, configHome, "jobs", "list", "--size", "3"); code != 0 {
		t.Fatalf("`jobs list --size` should work as --limit, exit %d: %s", code, stderr)
	}
	// --limit is jobs' spelling; video list says --size.
	if _, stderr, code := runVideoCmd(t, bin, configHome, "video", "list", "--limit", "3"); code != 0 {
		t.Fatalf("`video list --limit` should work as --size, exit %d: %s", code, stderr)
	}
	// --no-wait is auth login's spelling for "do not block".
	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_agent12345", "--kb-id", "kb_x", "--no-wait", "--output", "json")
	if code != 0 {
		t.Fatalf("`create --no-wait` should work as --async, exit %d: %s", code, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("bad json %q: %v", stdout, err)
	}
	if out["task_id"] == nil {
		t.Fatalf("async payload missing task_id: %v", out)
	}

	// Aliases must not appear in help — one name per concept for the reader.
	help, _, _ := runVideoCmd(t, bin, configHome, "jobs", "list", "--help")
	if strings.Contains(help, "--size") {
		t.Fatalf("the alias should not be documented as a second flag:\n%s", help)
	}
}

// TestAsyncPayloadGuidesNextStep: every other JSON payload carries
// next_actions, which is how a caller decides what to run next without having
// memorised the workflow. --async used to hand back two bare ids.
func TestAsyncPayloadGuidesNextStep(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := agentStub(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_agent12345", "--kb-id", "kb_x", "--async", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("bad json %q: %v", stdout, err)
	}
	acts, _ := out["next_actions"].([]any)
	if len(acts) == 0 {
		t.Fatalf("async payload must carry next_actions: %v", out)
	}
	first, _ := acts[0].(map[string]any)
	cmdStr, _ := first["command"].(string)
	if !strings.Contains(cmdStr, "video wait") {
		t.Fatalf("the next step after --async is `video wait`, got %q", cmdStr)
	}
	// It has to be runnable as printed, ids included.
	if !strings.Contains(cmdStr, "--session-id") {
		t.Fatalf("next_actions must be copy-pasteable: %q", cmdStr)
	}
}

// TestAuthFailureExitsThree: an expired credential is the most common
// recoverable failure, and the docs tell agents to re-authenticate on exit 3.
// Only `create` classified backend codes, so every other command exited 1 and
// that instruction never fired.
func TestAuthFailureExitsThree(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 40100, "message": "token expired"})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{
		"account": srv.URL, "vectoria": srv.URL, "figlens": srv.URL, "vibeknow": srv.URL,
	})

	for _, args := range [][]string{
		{"credits", "balance"},
		{"video", "list"},
		{"kb", "list"},
		{"voice", "list"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
			if code != 3 {
				t.Fatalf("an expired credential must exit 3 so the caller knows to re-authenticate, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
		})
	}
}

// TestRetryableAndBusinessExitCodes checks the rest of the code→exit contract
// now applies outside `create` too.
func TestCodeToExitContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	cases := []struct {
		name string
		code int
		want int
	}{
		{"rate limited is retryable", 42900, 4},
		{"server error is retryable", 50000, 4},
		{"insufficient credits is business", 100001, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"code": tc.code, "message": "x"})
			}))
			defer srv.Close()

			bin := build(t)
			configHome := buildProfile(t, map[string]string{"vibeknow": srv.URL})
			stdout, stderr, code := runVideoCmd(t, bin, configHome, "credits", "balance")
			if code != tc.want {
				t.Fatalf("backend code %d should exit %d, got %d\nstdout: %s\nstderr: %s", tc.code, tc.want, code, stdout, stderr)
			}
		})
	}
}

// TestCreate_NoTerminalEventIsNotSuccess is the `create`-side companion to
// TestVideoWait_NoEventsIsNotSuccess. The reattach path was fixed first; the
// primary path still exited 0 with an empty stdout when the stream closed
// without a terminal event — success reported for a run whose state is
// unknown and which may well still be going.
func TestCreate_NoTerminalEventIsNotSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 42, "session_id": "s_x", "work_id": 43, "v": 3},
			})
		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")
		case "/v1/agent3forVideo/stream":
			// Closes cleanly, but never reports a terminal state.
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_abc12345", "--kb-id", "kb_x")
	if code != 6 {
		t.Fatalf("create must exit 6 (state unknown) when no terminal event arrives, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "vk video wait") {
		t.Fatalf("the error must show how to reattach, got: %s", stderr)
	}
	// The run is recorded, so `jobs list --active` can still find it.
	jobsOut, _, jobsCode := runVideoCmd(t, bin, configHome, "jobs", "list", "--active", "--output", "json")
	if jobsCode != 0 {
		t.Fatalf("jobs list exit %d", jobsCode)
	}
	if !strings.Contains(jobsOut, "s_x") {
		t.Fatalf("an unresolved run must stay in the ledger as active: %s", jobsOut)
	}
}

// TestUnknownPipelineNodeStillReportsProgress: the backend has already
// renamed its pipeline graph once. Dropping node events this build does not
// recognize made the CLI go silent for the whole of that node — on an
// operation that runs for minutes. They are forwarded as free-form progress
// instead, without the raw wire name, which may carry an internal codename.
func TestUnknownPipelineNodeStillReportsProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 42, "session_id": "s_n", "work_id": 43, "v": 3},
			})
		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"a_node_from_the_future\",\"status\":\"start\",\"message\":\"still working\"}}}\n\n")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s_n\"}}\n\ndata: [DONE]\n\n")
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"id": 43, "session_id": "s_n", "title": "N",
					"share_token": "tok_n", "status": 1, "exporting": 0},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_abc12345", "--kb-id", "kb_x", "--output", "ndjson")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "still working") {
		t.Fatalf("progress from an unrecognized node must still reach the caller:\n%s", stdout)
	}
	if strings.Contains(stdout, "a_node_from_the_future") {
		t.Fatalf("the raw wire name must not be echoed:\n%s", stdout)
	}
}

// agentStub is a full happy-path figlens for the agent-facing tests.
func agentStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 4242, "session_id": "s_agent", "work_id": 77, "v": 3},
			})
		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"prepare\",\"status\":\"start\",\"message\":\"go\"}}}\n\n")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s_agent\"}}\n\ndata: [DONE]\n\n")
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"id": 77, "session_id": "s_agent", "title": "Agent",
					"share_token": "tok_agent", "status": 1, "exporting": 0},
			})
		case "/v1/works/page":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"list": []any{}, "total": 0},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}
