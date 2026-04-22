package exportpoll_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/video/exportpoll"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

type stubClient struct {
	seq []figlens.ExportResult
	i   int
	err error
}

func (s *stubClient) GetExportResult(ctx context.Context, _ string) (*figlens.ExportResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.i >= len(s.seq) {
		return &s.seq[len(s.seq)-1], nil
	}
	r := s.seq[s.i]
	s.i++
	return &r, nil
}

func TestPollExport_SucceedsOnCompleted(t *testing.T) {
	c := &stubClient{seq: []figlens.ExportResult{{Status: "completed", VideoPath: "v.mp4"}}}
	var events []exportpoll.Event
	r, err := exportpoll.PollExport(context.Background(), c, "exp", time.Minute, time.Nanosecond, func(e exportpoll.Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "completed" || r.VideoPath != "v.mp4" {
		t.Fatalf("result = %+v", r)
	}
	if len(events) != 1 || events[0].Status != snapshot.StatusSucceeded || events[0].Progress != 100 {
		t.Fatalf("events = %+v", events)
	}
}

func TestPollExport_TransitionsRunningThenSucceeded(t *testing.T) {
	c := &stubClient{seq: []figlens.ExportResult{
		{Status: "processing", Progress: 30, ProgressMsg: "frames"},
		{Status: "processing", Progress: 75, ProgressMsg: "audio"},
		{Status: "completed", VideoPath: "final.mp4"},
	}}
	var events []exportpoll.Event
	_, err := exportpoll.PollExport(context.Background(), c, "exp", time.Minute, time.Nanosecond, func(e exportpoll.Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if events[0].Status != snapshot.StatusRunning || events[0].Progress != 30 {
		t.Fatalf("event 0 = %+v", events[0])
	}
	if events[1].ProgressMsg != "audio" {
		t.Fatalf("event 1 msg = %q", events[1].ProgressMsg)
	}
	if events[2].Status != snapshot.StatusSucceeded {
		t.Fatalf("event 2 status = %q", events[2].Status)
	}
}

func TestPollExport_EmitsFailedEvent(t *testing.T) {
	c := &stubClient{seq: []figlens.ExportResult{{Status: "failed", Error: "render died"}}}
	var events []exportpoll.Event
	r, err := exportpoll.PollExport(context.Background(), c, "exp", time.Minute, time.Nanosecond, func(e exportpoll.Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "failed" {
		t.Fatalf("result status = %q", r.Status)
	}
	if len(events) != 1 || events[0].Status != snapshot.StatusFailed || events[0].ProgressMsg != "render died" {
		t.Fatalf("events = %+v", events)
	}
}

func TestPollExport_TimeoutReturnsErrTimeout(t *testing.T) {
	// Client keeps returning "running" forever; deadline fires first.
	c := &stubClient{seq: []figlens.ExportResult{{Status: "processing", Progress: 5}}}
	_, err := exportpoll.PollExport(context.Background(), c, "exp", 5*time.Millisecond, time.Millisecond, func(e exportpoll.Event) {})
	if !errors.Is(err, exportpoll.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestPollExport_ContextCancelled(t *testing.T) {
	c := &stubClient{seq: []figlens.ExportResult{{Status: "processing"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := exportpoll.PollExport(ctx, c, "exp", time.Minute, time.Millisecond, func(e exportpoll.Event) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
