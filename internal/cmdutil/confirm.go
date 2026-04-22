package cmdutil

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ConfirmOptions configures a [y/N] confirmation prompt. A prompt is shown
// only when the process is attached to a TTY AND --yes is not set AND
// VIBEKNOW_ASSUME_YES is not set. In any non-interactive context, Confirm
// returns (true, nil) so scripts and agents are never blocked.
type ConfirmOptions struct {
	Prompt string // shown to the user (written to Err)
	Yes    bool   // from --yes / -y flag
	In     io.Reader
	Err    io.Writer
	IsTTY  func() bool // defaults to stderr-is-terminal
}

func Confirm(opts ConfirmOptions) (bool, error) {
	if opts.Yes {
		return true, nil
	}
	if os.Getenv("VIBEKNOW_ASSUME_YES") != "" {
		return true, nil
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	if opts.IsTTY == nil {
		opts.IsTTY = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
	}
	if !opts.IsTTY() {
		return true, nil
	}
	fmt.Fprintf(opts.Err, "%s [y/N] ", opts.Prompt)
	line, err := bufio.NewReader(opts.In).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}
