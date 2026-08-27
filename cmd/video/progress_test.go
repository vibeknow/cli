package video

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vibeknow/cli/client/figlens"
)

func TestRunProgressDescribe(t *testing.T) {
	tests := []struct {
		desc   string
		mode   string
		events []figlens.StreamEvent
		want   string
	}{
		{
			desc: "nothing seen yet",
			want: "no stage reported yet",
		},
		{
			desc:   "stage and node together",
			events: []figlens.StreamEvent{{Type: "node.started", Stage: "outline", Node: "big_director"}},
			want:   "outline / big_director",
		},
		{
			// The image line's wire names carry an internal model codename;
			// DisplayName is what keeps it out of anything a user reads.
			desc:   "image node is shown under its sanitized name",
			events: []figlens.StreamEvent{{Type: "node.started", Stage: "outline", Node: "image2_theme_select"}},
			want:   "outline / style_select",
		},
		{
			desc: "latest position wins",
			events: []figlens.StreamEvent{
				{Type: "node.succeeded", Stage: "outline", Node: "script_writing"},
				{Type: "node.started", Stage: "tts", Node: "tts_generate"},
			},
			want: "tts / tts_generate",
		},
		{
			// The v=2 agent engine emits no step_ids at all, so its free-form
			// progress line is the only thing there is to report.
			desc:   "free-form progress when there is no node",
			events: []figlens.StreamEvent{{Type: "node.progress", Message: "正在生成分镜"}},
			want:   "正在生成分镜",
		},
		{
			// A node still open is the current position.
			desc:   "an open node is where the run is",
			events: []figlens.StreamEvent{{Type: "node.started", Stage: "render", Node: "bg_images"}},
			want:   "render / bg_images",
		},
		{
			// Reporting a finished node as the current one is how a run that
			// has moved on looks identical to one that has stopped.
			desc: "a finished node is reported as passed, not as current",
			events: []figlens.StreamEvent{
				{Type: "node.started", Stage: "outline", Node: "script_writing"},
				{Type: "node.succeeded", Stage: "outline", Node: "script_writing"},
			},
			want: "past script_writing",
		},
		{
			// The case that made this necessary: the hand-drawn line finishes
			// script_writing and then draws for minutes in silence. Repeating
			// "outline / script_writing" that whole time says the run is doing
			// something it finished long ago.
			desc: "hand-drawn silence after a finished node names both",
			mode: figlens.VideoKindHandDraw,
			events: []figlens.StreamEvent{
				{Type: "node.started", Stage: "outline", Node: "script_writing"},
				{Type: "node.succeeded", Stage: "outline", Node: "script_writing"},
			},
			want: "past script_writing; drawing (this line reports no progress until it finishes)",
		},
		{
			// A warning neither moves the run nor closes the node.
			desc: "a warning does not close the node",
			events: []figlens.StreamEvent{
				{Type: "node.started", Stage: "render", Node: "bg_images"},
				{Type: "node.warning", Stage: "render", Node: "bg_images", Message: "retrying one image"},
			},
			want: "render / bg_images",
		},
		{
			// The hand-drawn line registers no step_ids, so nothing arrives
			// between start and finish. Repeating "no stage reported yet" for
			// an hour is the same sentence a stuck run produces, on the one
			// line where the wait is longest — so the silence is named.
			desc: "hand-drawn silence is named as expected",
			mode: figlens.VideoKindHandDraw,
			want: "drawing (this line reports no progress until it finishes)",
		},
		{
			// Only the absence is explained. Once the line does report
			// something — its terminal nodes do — that is what gets shown.
			desc:   "a hand-drawn run that has reported something shows it",
			mode:   figlens.VideoKindHandDraw,
			events: []figlens.StreamEvent{{Type: "node.started", Stage: "publish", Node: "video_package"}},
			want:   "publish / video_package",
		},
		{
			// An unknown mode never becomes a claim about the run.
			desc: "an unrecorded mode says only what is known",
			mode: "",
			want: "no stage reported yet",
		},
		{
			// A node, once seen, outranks a progress line: it is the more
			// precise answer to "where is this".
			desc: "a node outranks a later free-form line",
			events: []figlens.StreamEvent{
				{Type: "node.started", Stage: "render", Node: "bg_images"},
				{Type: "node.progress", Message: "still working"},
			},
			want: "render / bg_images",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := runProgress{startedAt: time.Now()}
			for _, ev := range tt.events {
				p.observe(ev)
			}
			if got := p.describe(tt.mode); got != tt.want {
				t.Errorf("describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStillRunningErrorReportsAResumableRun(t *testing.T) {
	p := runProgress{startedAt: time.Now().Add(-95 * time.Second)}
	p.observe(figlens.StreamEvent{Type: "node.started", Stage: "tts", Node: "tts_generate"})

	err := stillRunningError(42, "s_abc", p, 90*time.Second)

	// Exit 6, not 0: `wait` exiting 0 means the task succeeded, and a caller
	// running `wait && download` must not be sent to download a video that is
	// still being made.
	if err.Code != 6 {
		t.Errorf("exit code = %d, want 6", err.Code)
	}
	if !strings.Contains(err.Message, "still generating") {
		t.Errorf("message should say the run is going, got: %q", err.Message)
	}

	detail, ok := err.Detail.(map[string]any)
	if !ok {
		t.Fatalf("detail is %T, want map", err.Detail)
	}

	// `reason` is what separates a spent budget from the two other things
	// exit 6 covers — a broken stream and a paused task — both of which are
	// states nobody asked for and need a different response.
	if detail["reason"] != "wait_budget_expired" {
		t.Errorf("reason = %v, want wait_budget_expired", detail["reason"])
	}
	if detail["status"] != "running" {
		t.Errorf("status = %v, want running", detail["status"])
	}
	if detail["stage"] != "tts / tts_generate" {
		t.Errorf("stage = %v, want the last position seen", detail["stage"])
	}
	if ms, _ := detail["waited_ms"].(int64); ms < 90_000 {
		t.Errorf("waited_ms = %v, want at least the budget", detail["waited_ms"])
	}

	// The next action has to be runnable as printed: a caller that has to
	// assemble the command itself is one that can get it wrong and start a
	// second billed run instead of reattaching to this one.
	actions, ok := detail["next_actions"].([]map[string]string)
	if !ok || len(actions) != 1 {
		t.Fatalf("next_actions = %#v, want exactly one", detail["next_actions"])
	}
	cmd := actions[0]["command"]
	for _, want := range []string{"vk video wait 42", "--session-id s_abc", "--for 1m30s"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("next action %q is missing %q", cmd, want)
		}
	}

	// The whole detail has to survive the JSON envelope, which is the only
	// form a scripted caller ever sees it in.
	if _, err := json.Marshal(detail); err != nil {
		t.Errorf("detail does not marshal: %v", err)
	}
}

// TestStillRunningErrorOnTheAgentLine covers the engine that reports no nodes
// at all: `--engine agent` emits free-form progress only, so the stage a
// caller repeats to a person is that line verbatim.
func TestStillRunningErrorOnTheAgentLine(t *testing.T) {
	p := runProgress{startedAt: time.Now().Add(-70 * time.Second)}
	p.observe(figlens.StreamEvent{Type: "node.progress", Message: "正在生成封面"})

	err := stillRunningError(7, "s_agent", p, 70*time.Second)
	detail, ok := err.Detail.(map[string]any)
	if !ok {
		t.Fatalf("detail is %T, want map", err.Detail)
	}
	if detail["stage"] != "正在生成封面" {
		t.Errorf("stage = %v, want the free-form line", detail["stage"])
	}
	// With no node, `stage` already is the free-form line. Emitting `message`
	// as well would put one string in the payload twice, which reads as two
	// facts and invites a caller to look for a difference between them.
	if _, present := detail["message"]; present {
		t.Errorf("message duplicates stage on this line: %#v", detail)
	}
}

// Where the two differ they must both survive: a node names the position, the
// free-form line says what is happening inside it.
func TestStillRunningErrorKeepsAMessageThatAddsSomething(t *testing.T) {
	p := runProgress{startedAt: time.Now().Add(-70 * time.Second)}
	p.observe(figlens.StreamEvent{Type: "node.started", Stage: "render", Node: "bg_images"})
	p.observe(figlens.StreamEvent{Type: "node.progress", Message: "第 3 张，共 8 张"})

	err := stillRunningError(7, "s_x", p, 70*time.Second)
	detail := err.Detail.(map[string]any)
	if detail["stage"] != "render / bg_images" {
		t.Errorf("stage = %v, want the node position", detail["stage"])
	}
	if detail["message"] != "第 3 张，共 8 张" {
		t.Errorf("message = %v, want the free-form detail kept", detail["message"])
	}
}
