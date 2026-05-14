package figlens

// Engine selects which go-figlens video generation pipeline to invoke.
// The zero value (EnginePipeline) preserves 0.4.2 behavior for callers
// that don't set Engine explicitly.
type Engine int

const (
	EnginePipeline Engine = 0 // → wire v=3, /agent3forVideo/stream
	EngineAgent    Engine = 1 // → wire v=2, /agent2forVideo/stream
)

// Wire returns the backend's "v" field value for this engine.
// 3 = pipeline (PipelineForVideo handler, WorkEngine="suite" in DB).
// 2 = agent    (AgentOnlyForVideo handler, WorkEngine="agent" in DB).
func (e Engine) Wire() int {
	switch e {
	case EngineAgent:
		return 2
	default:
		return 3
	}
}

// StreamPath returns the SSE endpoint path for this engine.
func (e Engine) StreamPath() string {
	switch e {
	case EngineAgent:
		return "/v1/agent2forVideo/stream"
	default:
		return "/v1/agent3forVideo/stream"
	}
}

// RemapEngineForDisplay translates the backend's Work.Engine DB enum
// value to the CLI's user-facing vocabulary used by --engine.
// Unknown values (including "agent" which is already user-facing)
// pass through unchanged.
func RemapEngineForDisplay(dbEnum string) string {
	if dbEnum == "suite" {
		return "pipeline"
	}
	return dbEnum
}
