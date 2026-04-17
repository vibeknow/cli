package clerr

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const (
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorReset  = "\033[0m"
)

func isTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// Render writes a formatted error to w. Includes hint if present.
func Render(w io.Writer, err error) {
	var clErr *Error
	switch e := err.(type) {
	case *Error:
		clErr = e
	default:
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}

	if isTTY() {
		fmt.Fprintf(w, "%sError%s: %s\n", colorRed, colorReset, clErr.Message)
		if clErr.Hint != "" {
			fmt.Fprintf(w, "%sHint%s:  %s\n", colorYellow, colorReset, clErr.Hint)
		}
	} else {
		fmt.Fprintf(w, "Error: %s\n", clErr.Message)
		if clErr.Hint != "" {
			fmt.Fprintf(w, "Hint: %s\n", clErr.Hint)
		}
	}
}
