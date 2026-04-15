package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectFormat(t *testing.T) {
	cases := []struct {
		flag, expected string
		isTTY          bool
		streaming      bool
	}{
		{"", "text", true, false},
		{"", "json", false, false},
		{"", "ndjson", false, true},
		{"json", "json", true, true},
		{"ndjson", "ndjson", false, false},
		{"text", "text", false, false},
	}
	for _, c := range cases {
		got := Select(c.flag, c.isTTY, c.streaming)
		if got != c.expected {
			t.Errorf("Select(%q, tty=%v, stream=%v) = %q, want %q",
				c.flag, c.isTTY, c.streaming, got, c.expected)
		}
	}
}

func TestTextStripsControl(t *testing.T) {
	var buf bytes.Buffer
	w := NewText(&buf)
	w.Print("clean ", "\x1b[31mred\x1b[0m", " tail")
	if got := buf.String(); got != "clean red tail" {
		t.Errorf("got %q", got)
	}
}

func TestJSONObject(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSON(&buf)
	w.Object(map[string]any{"k": "v", "n": 1})
	got := buf.String()
	if !strings.Contains(got, `"k":"v"`) || !strings.Contains(got, `"n":1`) || !strings.Contains(got, `"schema_version":"1"`) {
		t.Errorf("got %q", got)
	}
}

func TestNDJSONEvent(t *testing.T) {
	var buf bytes.Buffer
	w := NewNDJSON(&buf)
	w.Event(map[string]any{"event": "x", "task_id": "t_1"})
	w.Event(map[string]any{"event": "y", "task_id": "t_1"})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), buf.String())
	}
	for _, l := range lines {
		if !strings.Contains(l, `"schema_version":"1"`) || !strings.Contains(l, `"ts":`) {
			t.Errorf("line missing canonical fields: %q", l)
		}
	}
}

func TestUnsupportedFormat(t *testing.T) {
	if _, err := New("yaml", nil, false, false); err == nil {
		t.Error("expected error for unsupported format")
	}
}
