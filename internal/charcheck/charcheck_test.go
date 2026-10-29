package charcheck

import (
	"strings"
	"testing"
)

func TestStrip(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain ascii", "hello", "hello"},
		{"preserve tab/newline", "a\tb\nc", "a\tb\nc"},
		{"strip bare ESC", "a\x1bb", "ab"},
		{"strip CSI color", "\x1b[31mred\x1b[0m", "red"},
		{"strip carriage return overwrite", "progress\r100%", "progress100%"},
		{"strip C1", "a\x9bb", "ab"},
		{"preserve utf8", "你好\n世界", "你好\n世界"},
		{"strip OSC", "\x1b]0;title\x07rest", "rest"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Strip(c.in)
			if got != c.want {
				t.Fatalf("Strip(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNewWriterStrips(t *testing.T) {
	var b strings.Builder
	w := NewWriter(&b)
	n, err := w.Write([]byte("a\x1b[31mb\tc\n"))
	if err != nil || n != len("a\x1b[31mb\tc\n") {
		t.Fatalf("Write = %d, %v; want the full input length", n, err)
	}
	if got := b.String(); got != "ab\tc\n" {
		t.Errorf("got %q", got)
	}
}
