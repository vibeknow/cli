package cmd

import (
	"strings"
	"testing"
)

func TestResolveVideoKind(t *testing.T) {
	tests := []struct {
		flag    string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"replica", "replica", false},
		{"script", "script_lock", false},
		{"SCRIPT", "script_lock", false},
		{"script_lock", "", true}, // backend jargon, not a CLI flag value
		{"bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveVideoKind(tt.flag)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.flag)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveVideoKind(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestResolveAspect(t *testing.T) {
	tests := []struct {
		flag    string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"horizontal", "horizontal", false},
		{"vertical", "vertical", false},
		{"16:9", "horizontal", false},
		{"9:16", "vertical", false},
		{"HORIZONTAL", "horizontal", false},
		{"square", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveAspect(tt.flag)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.flag)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveAspect(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestResolveVideoKind_ErrorMessageMentionsValues(t *testing.T) {
	_, err := resolveVideoKind("xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "replica") || !strings.Contains(msg, "script") {
		t.Fatalf("error must list allowed values, got: %q", msg)
	}
}

