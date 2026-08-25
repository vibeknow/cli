package cmdutil

import (
	"context"
	"errors"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/errs"
)

type stubWork struct {
	work *figlens.Work
	err  error
}

func (s stubWork) GetWorkBySession(context.Context, string) (*figlens.Work, error) {
	return s.work, s.err
}

func TestProbeRun(t *testing.T) {
	cases := []struct {
		name  string
		stub  stubWork
		want  string
		safe  bool
		why   string
		state string
	}{
		{
			name:  "backend has the run",
			stub:  stubWork{work: &figlens.Work{ID: 7, Status: figlens.WorkStatusGenerating}},
			want:  DeliverySubmitted,
			safe:  false,
			state: "running",
			why:   "a second `vk create` here is a second billed render",
		},
		{
			name: "backend never heard of it",
			stub: stubWork{err: &errs.Object{Code: "not_found", Message: "no work"}},
			want: DeliveryNotSubmitted,
			safe: true,
			why:  "the work row is created at init; its absence means nothing was billed",
		},
		{
			name: "empty row counts as never heard of it",
			stub: stubWork{work: &figlens.Work{}},
			want: DeliveryNotSubmitted,
			safe: true,
		},
		{
			name: "backend unreachable",
			stub: stubWork{err: errors.New("dial tcp: connection refused")},
			want: DeliveryIndeterminate,
			safe: false,
			why:  "waiting when a run was lost costs a delay; resending when it was not costs the user's money",
		},
		{
			name:  "already finished",
			stub:  stubWork{work: &figlens.Work{ID: 7, Status: figlens.WorkStatusActive}},
			want:  DeliverySubmitted,
			safe:  false,
			state: "succeeded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProbeRun(context.Background(), tc.stub, 42, "sess_1")
			if got.Delivery != tc.want {
				t.Fatalf("delivery = %q, want %q. %s", got.Delivery, tc.want, tc.why)
			}
			if got.ResendSafe() != tc.safe {
				t.Fatalf("ResendSafe() = %v, want %v. %s", got.ResendSafe(), tc.safe, tc.why)
			}
			if tc.state != "" && got.BackendStatus != tc.state {
				t.Fatalf("backend_status = %q, want %q", got.BackendStatus, tc.state)
			}
		})
	}
}

func TestProbeRun_NoSessionIsIndeterminate(t *testing.T) {
	got := ProbeRun(context.Background(), stubWork{work: &figlens.Work{ID: 1}}, 42, "")
	if got.Delivery != DeliveryIndeterminate {
		t.Fatalf("without a session there is nothing to look up; got %q", got.Delivery)
	}
}

func TestProbeRun_NilClientDoesNotPanic(t *testing.T) {
	if got := ProbeRun(context.Background(), nil, 42, "s"); got.Delivery != DeliveryIndeterminate {
		t.Fatalf("delivery = %q, want %q", got.Delivery, DeliveryIndeterminate)
	}
}

func TestUnknownStateError_CarriesTheVerdictAsData(t *testing.T) {
	u := UnknownRun{TaskID: 42, SessionID: "sess_1", Delivery: DeliverySubmitted, BackendStatus: "running"}
	err := UnknownStateError("stream ended early", u, "vk video wait 42 --session-id sess_1")

	if clerr.ExitCodeFor(err) != 6 {
		t.Fatalf("exit code = %d, want 6", clerr.ExitCodeFor(err))
	}
	d, ok := err.Detail.(map[string]any)
	if !ok {
		t.Fatalf("detail must be structured, got %T", err.Detail)
	}
	// resend_safe is the field a small model can branch on without
	// interpreting anything. It is the whole point of the three states.
	if d["resend_safe"] != false {
		t.Fatalf("resend_safe = %v, want false for a submitted run", d["resend_safe"])
	}
	if d["delivery"] != DeliverySubmitted {
		t.Fatalf("delivery = %v", d["delivery"])
	}
	if d["task_id"] != int64(42) || d["session_id"] != "sess_1" {
		t.Fatalf("the ids needed to reattach are missing: %v", d)
	}
	if d["backend_status"] != "running" {
		t.Fatalf("backend_status = %v", d["backend_status"])
	}
	actions, ok := d["next_actions"].([]map[string]string)
	if !ok || len(actions) != 1 {
		t.Fatalf("next_actions = %v, want exactly one runnable command", d["next_actions"])
	}
	if actions[0]["command"] != "vk video wait 42 --session-id sess_1" {
		t.Fatalf("command = %q", actions[0]["command"])
	}
	if err.Hint != actions[0]["command"] {
		t.Fatal("prose and JSON callers must be pointed at the same command")
	}
}

func TestUnknownStateError_OmitsWhatItDoesNotKnow(t *testing.T) {
	err := UnknownStateError("no idea", UnknownRun{Delivery: DeliveryIndeterminate}, "")
	d := err.Detail.(map[string]any)
	for _, k := range []string{"task_id", "session_id", "backend_status", "next_actions"} {
		if _, ok := d[k]; ok {
			t.Fatalf("%q present but unknown; an empty value reads as a real one", k)
		}
	}
	if d["resend_safe"] != false {
		t.Fatal("indeterminate must never say a resend is safe")
	}
}

func TestWorkStatusName(t *testing.T) {
	cases := map[int]string{
		figlens.WorkStatusGenerating: "running",
		figlens.WorkStatusActive:     "succeeded",
		figlens.WorkStatusFailed:     "failed",
		figlens.WorkStatusDeleted:    "deleted",
		99:                           "unknown",
	}
	for in, want := range cases {
		if got := workStatusName(in); got != want {
			t.Fatalf("workStatusName(%d) = %q, want %q", in, got, want)
		}
	}
}
