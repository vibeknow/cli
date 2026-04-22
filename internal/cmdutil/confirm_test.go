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

func TestConfirm_NonTTYSkipsPrompt(t *testing.T) {
	var out bytes.Buffer
	ok, err := cmdutil.Confirm(cmdutil.ConfirmOptions{
		Prompt: "do it?",
		In:     strings.NewReader(""),
		Err:    &out,
		IsTTY:  func() bool { return false },
	})
	if err != nil || !ok {
		t.Fatalf("non-TTY should skip prompt; got (%v,%v)", ok, err)
	}
	if out.Len() != 0 {
		t.Fatalf("should not write prompt in non-TTY: %q", out.String())
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
