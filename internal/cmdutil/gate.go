package cmdutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/pending"
)

// ExitBlocked is the exit code for a command that stopped at a user
// decision it is not allowed to make.
//
// It is deliberately not 0. Flova, which this borrows from, exits 0 here on
// the reasoning that the run legitimately completed at a boundary — but
// their contract puts the outcome in an envelope the caller must parse, and
// ours puts it in the exit code precisely so a small model does not have to
// parse anything. Under our contract `vk video export` exiting 0 states
// that the MP4 exists. At this boundary it does not, so 0 would be a lie in
// the one channel we have spent the most effort making trustworthy.
//
// It is also not 2. A blocked command is not malformed — there is nothing
// wrong with the command line, and telling an agent to fix its arguments
// would send it looking for a mistake it did not make.
const ExitBlocked = 8

// ExportActionType names the MP4 spend boundary.
//
// It lives here rather than in either command because `vk create --export`
// and `vk video export` are the same decision reached two ways. Minting the
// token from a shared definition is what lets a caller blocked by the first
// resume with the second — the alternative is a token that verifies only
// against the command that issued it, which would force an agent to
// remember which path it came from.
//
// Renaming it invalidates every outstanding confirmation, which is correct
// if what the boundary means ever changes.
const ExportActionType = "export_confirmation"

// ExportActionPayload is what the user is being asked to agree to.
//
// session_id is in here so a token minted for one run cannot authorise
// another: an agent holding a confirmation from an earlier video must not
// be able to spend a credit on a different one.
func ExportActionPayload(sessionID string) map[string]any {
	return map[string]any{
		"session_id": sessionID,
		"credits":    1,
		"operation":  "render_mp4",
	}
}

// SceneEditActionType names the boundary for rewriting a shot's narration.
//
// Separate from ExportActionType because they are different decisions with
// different costs, and a token minted for one must not authorise the other.
const SceneEditActionType = "scene_edit_confirmation"

// SceneEditActionPayload is what the user is being asked to agree to.
//
// The old and new narration are both in here, in full, and both are hashed
// into the action_id. That is the point rather than a side effect:
//
//   - The user is agreeing to a *diff*, so both halves have to be visible.
//     A payload carrying only the replacement asks them to approve a change
//     they cannot see.
//   - Consent is to one specific rewrite. An agent iterating on wording
//     cannot carry a token from the last attempt to the next one, because
//     the new text is different and the token no longer verifies.
//   - If the work moved underneath — someone else edited this shot between
//     the block and the resume — `from` no longer matches, the token stops
//     verifying, and the caller is sent back to look at the current text.
//     Without this, a stale confirmation would overwrite an edit nobody
//     asked to discard.
//
// script_only is in here too: it decides what is billed, so it decides what
// is being consented to.
func SceneEditActionPayload(sessionID string, sceneIndex int, from, to string, scriptOnly bool) map[string]any {
	return map[string]any{
		"session_id":  sessionID,
		"scene_index": sceneIndex,
		"operation":   "edit_script",
		"script_only": scriptOnly,
		"from":        from,
		"to":          to,
	}
}

// GateOptions describes a spend the user has to authorise.
type GateOptions struct {
	// Type is the stable boundary token, e.g. "export_confirmation".
	Type string
	// Payload carries the decision-relevant facts. It is shown to the user
	// and hashed into the action_id, so anything that changes what is being
	// agreed to belongs here.
	Payload map[string]any
	// Prompt is the human sentence: what is about to happen and what it costs.
	Prompt string
	// Yes is the --yes flag: the caller states it already has authority.
	Yes bool
	// Token is the --confirm value: authority obtained through this gate.
	Token string
	// ResumeCommand is the exact command line that proceeds, with the token
	// already in it.
	ResumeCommand func(token string) string
	// IsTTY overrides terminal detection in tests.
	IsTTY func() bool
}

// Gate decides whether a paid action may proceed.
//
// The four paths, in order:
//
//	--yes / VIBEKNOW_ASSUME_YES  proceed; the caller has taken responsibility
//	--confirm <token>            proceed; authority came from this gate
//	a terminal                   ask, the way a person expects to be asked
//	anything else                stop, and hand back the decision
//
// The last line is the change. It used to proceed with a note on stderr,
// which meant an agent could spend credits it was never told to spend and
// the only trace was a line nobody was reading.
//
// Returns (true, nil) to proceed, (false, nil) when a person declined at
// the prompt, and (false, err) with ExitBlocked when the decision was
// handed back. In the blocked case the action has already been written to
// stdout: it is the command's result, not an error report, so it belongs in
// the channel the caller reads results from.
func Gate(cmd *cobra.Command, opts GateOptions) (bool, error) {
	if opts.Yes || os.Getenv("VIBEKNOW_ASSUME_YES") != "" {
		return true, nil
	}

	if opts.Token != "" {
		if pending.Verify(opts.Token, opts.Type, opts.Payload) {
			return true, nil
		}
		// Distinguishing a stale token from a wrong one is not worth it:
		// both mean "the terms you agreed to are not the terms on offer",
		// and both are fixed the same way — re-run, re-ask, re-pass.
		return false, clerr.Validation("--confirm token does not match this action").
			WithHint("re-run the command without --confirm to get the current action_id, show the user what it costs, and pass that value back")
	}

	if opts.IsTTY == nil {
		opts.IsTTY = defaultIsTTY
	}
	if opts.IsTTY() {
		return Confirm(ConfirmOptions{Prompt: opts.Prompt, Yes: false})
	}

	return false, block(cmd, opts)
}

// block mints the token, writes the action, and returns the ExitBlocked error.
func block(cmd *cobra.Command, opts GateOptions) error {
	token, err := pending.Token(opts.Type, opts.Payload)
	if err != nil {
		// Without a token there is no way to authorise this run at all, and
		// falling through to "proceed" would reintroduce exactly the silent
		// spend this exists to stop. Fail closed and name the escape hatch.
		return clerr.New(fmt.Sprintf("cannot mint a confirmation token: %v", err)).
			WithCode(ExitBlocked).
			WithHint("pass --yes (or set VIBEKNOW_ASSUME_YES=1) if you already have the user's authority for this spend")
	}

	resume := ""
	if opts.ResumeCommand != nil {
		resume = opts.ResumeCommand(token)
	}
	action := pending.Action{
		ActionID: token,
		Type:     opts.Type,
		Blocking: true,
		Message:  opts.Prompt,
		Payload:  opts.Payload,
		Options: []pending.Option{
			{ID: "confirm", Effect: pending.EffectResume, Label: "Proceed and spend the credits"},
			{ID: "cancel", Effect: pending.EffectNone, Label: "Do not proceed; run nothing"},
		},
		ResumeCommand: resume,
	}

	payload := map[string]any{
		"status":          "blocked",
		"pending_actions": []map[string]any{action.Map()},
	}
	format, _ := cmd.Flags().GetString("output")
	stdout := cmd.OutOrStdout()
	switch format {
	case output.FormatJSON:
		_ = output.NewJSON(stdout).Object(payload)
	case output.FormatNDJSON:
		payload["type"] = "action.required"
		_ = output.NewNDJSON(stdout).Event(payload)
	default:
		fmt.Fprintf(stdout, "status=blocked\naction_id=%s\n", token)
		if resume != "" {
			fmt.Fprintf(stdout, "resume=%s\n", resume)
		}
	}

	e := clerr.New(opts.Prompt).WithCode(ExitBlocked).WithType("blocked")
	if resume != "" {
		return e.WithHintf("ask the user, then run: %s", resume)
	}
	return e.WithHint("ask the user, then re-run with the action_id printed above")
}
