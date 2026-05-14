package kb

import (
	"testing"
	"time"
)

func TestFilterKBs_Pattern(t *testing.T) {
	items := []kbItem{
		{Name: "vibeknow-cli-1"},
		{Name: "manual-kb"},
		{Name: "vibeknow-cli-2"},
	}
	got, err := filterKBs(items, "vibeknow-cli-*", 0, time.Time{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("matched %d, want 2", len(got))
	}
}

func TestFilterKBs_Age(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	items := []kbItem{
		{Name: "old", CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{Name: "fresh", CreatedAt: now.Add(-3 * 24 * time.Hour)},
	}
	got, err := filterKBs(items, "", 7*24*time.Hour, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "old" {
		t.Fatalf("got %v", got)
	}
}

func TestFilterKBs_BadPattern(t *testing.T) {
	if _, err := filterKBs([]kbItem{{Name: "x"}}, "[unterminated", 0, time.Time{}); err == nil {
		t.Fatal("expected error on bad pattern")
	}
}

func TestFilterKBs_Combined(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	items := []kbItem{
		{Name: "vibeknow-cli-old", CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{Name: "vibeknow-cli-fresh", CreatedAt: now.Add(-3 * 24 * time.Hour)},
		{Name: "manual-old", CreatedAt: now.Add(-10 * 24 * time.Hour)},
	}
	got, err := filterKBs(items, "vibeknow-cli-*", 7*24*time.Hour, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "vibeknow-cli-old" {
		t.Fatalf("want only vibeknow-cli-old, got %v", got)
	}
}
