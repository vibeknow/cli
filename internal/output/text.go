package output

import (
	"fmt"
	"io"

	"github.com/shiliu-ai/vibeknow-cli/internal/charcheck"
)

type textW struct{ w io.Writer }

func NewText(w io.Writer) *textW { return &textW{w: w} }

func (t *textW) Format() string { return "text" }

// Print writes args concatenated, stripping control chars.
func (t *textW) Print(args ...any) {
	s := fmt.Sprint(args...)
	_, _ = io.WriteString(t.w, charcheck.Strip(s))
}

func (t *textW) Printf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	_, _ = io.WriteString(t.w, charcheck.Strip(s))
}

func (t *textW) Println(args ...any) {
	t.Print(args...)
	_, _ = io.WriteString(t.w, "\n")
}
