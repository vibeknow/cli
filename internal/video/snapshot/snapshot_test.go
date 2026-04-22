package snapshot_test

import (
	"testing"

	"github.com/vibeknow/cli/internal/video/snapshot"
)

func TestShareURL(t *testing.T) {
	cases := []struct {
		name, base, token, want string
	}{
		{"default base", "", "abc123", "https://vibeknow.com/share/abc123"},
		{"custom base", "https://self.example/s", "abc123", "https://self.example/s/abc123"},
		{"trailing slash stripped", "https://self.example/s/", "abc123", "https://self.example/s/abc123"},
		{"empty token returns empty", "https://self.example/s", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := snapshot.ShareURL(tc.base, tc.token)
			if got != tc.want {
				t.Fatalf("ShareURL(%q, %q) = %q, want %q", tc.base, tc.token, got, tc.want)
			}
		})
	}
}
