package cmd

import (
	"strings"
	"testing"
)

func TestPastedTextName(t *testing.T) {
	tests := []struct {
		desc string
		text string
		want string
	}{
		{desc: "opening line becomes the name", text: "季度复盘要点\n\n第一，收入…", want: "季度复盘要点.md"},
		{desc: "single line", text: "How caching works", want: "How caching works.md"},

		// A paste very often starts as markdown, and carrying the syntax into
		// a filename names the document after punctuation.
		{desc: "markdown heading loses its hashes", text: "## 三季度总结\n正文", want: "三季度总结.md"},
		{desc: "bullet loses its marker", text: "- 第一点\n- 第二点", want: "第一点.md"},

		// Path separators in a filename are the one thing that cannot be
		// passed through unchanged.
		{desc: "slashes are replaced", text: "2026/Q3 收入/成本", want: "2026-Q3 收入-成本.md"},
		{desc: "backslashes are replaced", text: `C:\notes`, want: "C--notes.md"},

		// Rich-text sources bring control characters along.
		{desc: "control characters are dropped", text: "标题\u0007文字", want: "标题文字.md"},
		{desc: "CRLF ends the line", text: "标题\r\n正文", want: "标题.md"},

		{desc: "long line is truncated to 40 runes", text: strings.Repeat("字", 60), want: strings.Repeat("字", 40) + ".md"},

		// Truncation counts runes, not bytes: cutting mid-character would
		// produce a name the user cannot read.
		{desc: "truncation does not split a rune", text: strings.Repeat("漢", 45), want: strings.Repeat("漢", 40) + ".md"},

		{desc: "nothing usable falls back", text: "###", want: "pasted-text.md"},
		{desc: "punctuation-only line falls back", text: "---\n正文", want: "pasted-text.md"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := pastedTextName(strings.TrimSpace(tt.text)); got != tt.want {
				t.Errorf("pastedTextName(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestUploadTextRejectsUnusableInput(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		restore := setCreateText("")
		defer restore()
		if _, _, err := uploadText(t.Context(), ""); err == nil {
			t.Fatal("expected empty text to be refused")
		}
	})

	t.Run("whitespace only", func(t *testing.T) {
		// Refused for the same reason as empty: the backend would accept the
		// upload, parse it to nothing, and the run would die several billed
		// steps later with a message about the document rather than the input.
		if _, _, err := uploadText(t.Context(), "  \n\t\n  "); err == nil {
			t.Fatal("expected whitespace-only text to be refused")
		}
	})

	t.Run("over the size limit", func(t *testing.T) {
		_, _, err := uploadText(t.Context(), strings.Repeat("x", maxPastedTextBytes+1))
		if err == nil {
			t.Fatal("expected oversized text to be refused")
		}
		if !strings.Contains(err.Error(), "over the") {
			t.Errorf("error should name the limit, got: %v", err)
		}
	})
}

func TestJobSourceLabelsThePaste(t *testing.T) {
	tests := []struct {
		desc string
		from string
		text string
		want string
	}{
		{desc: "a file keeps its path", from: "./report.pdf", want: "./report.pdf"},
		{desc: "a URL keeps itself", from: "https://example.com/a", want: "https://example.com/a"},
		{desc: "a paste shows its opening words", text: "季度复盘要点\n正文", want: "text: 季度复盘要点"},

		// stdin arrives as `--from -`, which is not a source anyone could
		// recognise in `vk jobs list`; by then the text has been read onto
		// the same field, so it is labelled like any other paste.
		{desc: "stdin is labelled by its text", from: "-", text: "从标准输入来的", want: "text: 从标准输入来的"},
		{desc: "unusable text still gets a label", from: "-", text: "###", want: "text: pasted-text"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			restoreFrom := setCreateFrom(tt.from)
			defer restoreFrom()
			restoreText := setCreateText(tt.text)
			defer restoreText()

			if got := jobSource(); got != tt.want {
				t.Errorf("jobSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func setCreateFrom(v string) func() {
	prev := flagCreateFrom
	flagCreateFrom = v
	return func() { flagCreateFrom = prev }
}

func setCreateText(v string) func() {
	prev := flagCreateText
	flagCreateText = v
	return func() { flagCreateText = prev }
}
