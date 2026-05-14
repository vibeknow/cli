package figlens_test

import (
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestEngineWireValue(t *testing.T) {
	if got := figlens.EnginePipeline.Wire(); got != 3 {
		t.Fatalf("EnginePipeline.Wire() = %d, want 3", got)
	}
	if got := figlens.EngineAgent.Wire(); got != 2 {
		t.Fatalf("EngineAgent.Wire() = %d, want 2", got)
	}
}

func TestEngineDefault(t *testing.T) {
	var zero figlens.Engine
	if zero != figlens.EnginePipeline {
		t.Fatalf("zero Engine = %v, want EnginePipeline (so InitTask{}/StreamParams{} stay backward-compat)", zero)
	}
}

func TestEngineStreamPath(t *testing.T) {
	if got := figlens.EnginePipeline.StreamPath(); got != "/v1/agent3forVideo/stream" {
		t.Fatalf("EnginePipeline.StreamPath() = %q, want /v1/agent3forVideo/stream", got)
	}
	if got := figlens.EngineAgent.StreamPath(); got != "/v1/agent2forVideo/stream" {
		t.Fatalf("EngineAgent.StreamPath() = %q, want /v1/agent2forVideo/stream", got)
	}
}

func TestRemapEngineForDisplay(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"suite", "pipeline"},
		{"agent", "agent"},
		{"", ""},
		{"unknown_future_engine", "unknown_future_engine"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := figlens.RemapEngineForDisplay(tt.in); got != tt.want {
				t.Fatalf("RemapEngineForDisplay(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
