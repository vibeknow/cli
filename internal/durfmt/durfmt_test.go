package durfmt_test

import (
	"testing"
	"time"

	"github.com/vibeknow/cli/internal/durfmt"
)

func TestParseAge(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"1h30m", time.Hour + 30*time.Minute},
		{"30m", 30 * time.Minute},
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := durfmt.ParseAge(tt.in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAge_Invalid(t *testing.T) {
	for _, bad := range []string{"", "abc", "7", "d", "7days"} {
		if _, err := durfmt.ParseAge(bad); err == nil {
			t.Errorf("ParseAge(%q) = nil err, want error", bad)
		}
	}
}
