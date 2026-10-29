package snapshot_test

import (
	"testing"

	"github.com/vibeknow/cli/internal/video/snapshot"
)

func TestTargetQuotesUnsafeSessionIDs(t *testing.T) {
	cases := []struct {
		task int64
		sess string
		want string
	}{
		{42, "39e730ed-3981-4873-bc63-8ff7167213df", "42 --session-id 39e730ed-3981-4873-bc63-8ff7167213df"},
		{0, "s_x", "--session-id s_x"},
		{42, "s; rm -rf ~", "42 --session-id 's; rm -rf ~'"},
		{0, "it's", `--session-id 'it'\''s'`},
	}
	for _, c := range cases {
		if got := snapshot.Target(c.task, c.sess); got != c.want {
			t.Errorf("Target(%d, %q) = %q, want %q", c.task, c.sess, got, c.want)
		}
	}
}
