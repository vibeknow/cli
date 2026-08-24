package cmdutil_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vibeknow/cli/internal/cmdutil"
)

func TestConfirm_YesFlagSkipsPrompt(t *testing.T) {
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		Yes:    true,
	})
	if err != nil || !ok {
		t.Fatalf("yes=true should skip prompt and return (true,nil); got (%v,%v)", ok, err)
	}
}

func TestConfirm_EnvSkipsPrompt(t *testing.T) {
	t.Setenv("VIBEKNOW_ASSUME_YES", "1")
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{Prompt: "do it?"})
	if err != nil || !ok {
		t.Fatalf("env should skip prompt; got (%v,%v)", ok, err)
	}
}

// TestConfirm_NonTTYProceedsAndSaysSo: a non-interactive caller cannot answer
// a prompt, so Confirm proceeds — but it has to leave a trace. Some of these
// prompts gate billed operations, and auto-confirming them in silence left no
// way to tell afterwards that a gate had been skipped at all.
func TestConfirm_NonTTYProceedsAndSaysSo(t *testing.T) {
	var out bytes.Buffer
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		In:     strings.NewReader(""),
		Err:    &out,
		IsTTY:  func() bool { return false },
	})
	if err != nil || !ok {
		t.Fatalf("non-TTY must proceed rather than block; got (%v,%v)", ok, err)
	}
	got := out.String()
	if !strings.Contains(got, "do it?") {
		t.Fatalf("the note should repeat what was auto-confirmed: %q", got)
	}
	if !strings.Contains(got, "--yes") {
		t.Fatalf("the note should say how to make the choice explicit: %q", got)
	}
	// It must never look like a question that was actually asked.
	if strings.Contains(got, "[y/N]") {
		t.Fatalf("non-TTY must not render a prompt: %q", got)
	}
}

// An explicit --yes is a decision the caller already made; it needs no note.
func TestConfirm_ExplicitYesIsSilent(t *testing.T) {
	var out bytes.Buffer
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		Yes:    true,
		In:     strings.NewReader(""),
		Err:    &out,
		IsTTY:  func() bool { return false },
	})
	if err != nil || !ok {
		t.Fatalf("--yes should confirm; got (%v,%v)", ok, err)
	}
	if out.Len() != 0 {
		t.Fatalf("--yes needs no explanation: %q", out.String())
	}
}

func TestConfirm_TTYYesAnswer(t *testing.T) {
	var out bytes.Buffer
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		In:     strings.NewReader("y\n"),
		Err:    &out,
		IsTTY:  func() bool { return true },
	})
	if err != nil || !ok {
		t.Fatalf("y answer should confirm; got (%v,%v)", ok, err)
	}
	if !strings.Contains(out.String(), "do it?") {
		t.Fatalf("prompt not written: %q", out.String())
	}
}

func TestConfirm_TTYNoAnswer(t *testing.T) {
	var out bytes.Buffer
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		In:     strings.NewReader("n\n"),
		Err:    &out,
		IsTTY:  func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("n answer should decline")
	}
}

func TestConfirm_TTYEmptyAnswerDefaultsNo(t *testing.T) {
	ok, _ := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		In:     strings.NewReader("\n"),
		Err:    &bytes.Buffer{},
		IsTTY:  func() bool { return true },
	})
	if ok {
		t.Fatal("empty answer should default to no")
	}
}
