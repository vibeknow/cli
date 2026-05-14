package kb

import "testing"

func TestPruneRequiresFilter(t *testing.T) {
	if err := validatePruneFilters("", ""); err == nil {
		t.Fatal("expected error when neither --pattern nor --older-than set")
	}
}

func TestPruneAcceptsEitherFilter(t *testing.T) {
	if err := validatePruneFilters("vibeknow-cli-*", ""); err != nil {
		t.Fatalf("pattern-only should be valid, got: %v", err)
	}
	if err := validatePruneFilters("", "7d"); err != nil {
		t.Fatalf("age-only should be valid, got: %v", err)
	}
	if err := validatePruneFilters("vibeknow-cli-*", "7d"); err != nil {
		t.Fatalf("both filters should be valid, got: %v", err)
	}
}
