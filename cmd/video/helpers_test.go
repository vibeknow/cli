package video

import "testing"

func TestTargetArgsOmitsUnknownTask(t *testing.T) {
	if got := targetArgs(42, "s_x"); got != "42 --session-id s_x" {
		t.Errorf("with task: got %q", got)
	}
	if got := targetArgs(0, "s_x"); got != "--session-id s_x" {
		t.Errorf("without task: got %q", got)
	}
}
