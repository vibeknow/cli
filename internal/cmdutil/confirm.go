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

// defaultIsTTY reports whether a person is watching. stderr rather than
// stdin, because stdout is routinely redirected by callers who are still
// sitting at the terminal.
func defaultIsTTY() bool { return term.IsTerminal(int(os.Stderr.Fd())) }

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
		opts.IsTTY = defaultIsTTY
	}
	if !opts.IsTTY() {
		// Proceeding is the right call — an agent or CI job cannot answer a
		// prompt, and blocking forever is the worse failure. But doing it in
		// total silence meant a billed action could happen with no gate and
		// no trace, so say what was assumed and how to make it explicit.
		fmt.Fprintf(opts.Err, "%s — no TTY, proceeding without confirmation (pass --yes to make this explicit)\n", opts.Prompt)
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
