package redact

import "testing"

func TestRedact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Authorization: Bearer abcdef1234567890", "Authorization: Bearer ***"},
		{"authorization: bearer  xyz", "authorization: bearer ***"},
		{"Cookie: session=abc123def; user=bob", "Cookie: session=***; user=bob"},
		{"X-Api-Key: sk_live_abcDEF123", "X-Api-Key: ***"},
		{"unrelated text", "unrelated text"},
		{"Basic dXNlcjpwYXNz", "Basic ***"},
	}
	for _, c := range cases {
		got := String(c.in)
		if got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
