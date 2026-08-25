// Package pending turns a spend decision into a protocol state instead of a
// terminal prompt.
//
// A confirmation prompt assumes someone is there to answer it. When an agent
// runs the CLI nobody is, and both available answers are bad: block forever,
// or spend the user's credits on their behalf and mention it afterwards. The
// CLI did the second one.
//
// The way out is to stop treating consent as an interaction and start
// treating it as a value. A blocked command returns the decision — what it
// costs, what the options are — plus an opaque token, and refuses to
// proceed without that token coming back. The agent's job becomes relaying
// a question and a token, which is a thing a small model does reliably;
// deciding on the user's behalf stops being reachable by accident, because
// there is no argument it can guess that means yes.
//
// The token is not a security boundary. Anything running as the user can
// read the key that mints it, and this package does not pretend otherwise.
// What it is is an *evidence* boundary: the token cannot be derived by
// reasoning about the conversation, so a model that produces one has been
// through the code path that disclosed the cost. That is the failure this
// prevents — confident invention, not a determined bypass.
package pending

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vibeknow/cli/internal/config"
)

// Effects a caller can dispatch for an option.
const (
	// EffectResume: re-run the command with the token to proceed.
	EffectResume = "resume"
	// EffectNone: do nothing. No command is run.
	EffectNone = "none"
)

// TokenPrefix marks a value as one of ours, so a caller passing something
// else gets told what is wrong rather than a bare mismatch.
const TokenPrefix = "act_"

// Option is one thing the user may choose.
type Option struct {
	ID     string `json:"id"`
	Effect string `json:"effect"`
	Label  string `json:"label"`
}

// Action is a boundary only the user can cross.
type Action struct {
	ActionID string         `json:"action_id"`
	Type     string         `json:"type"`
	Blocking bool           `json:"blocking"`
	Message  string         `json:"message"`
	Payload  map[string]any `json:"payload,omitempty"`
	Options  []Option       `json:"options"`
	// ResumeCommand is the exact command line that proceeds. It is spelled
	// out rather than described because the caller reconstructing it from
	// prose is the step that goes wrong.
	ResumeCommand string `json:"resume_command"`
}

// Map renders the action for a JSON payload.
func (a Action) Map() map[string]any {
	opts := make([]map[string]any, 0, len(a.Options))
	for _, o := range a.Options {
		opts = append(opts, map[string]any{"id": o.ID, "effect": o.Effect, "label": o.Label})
	}
	m := map[string]any{
		"action_id":      a.ActionID,
		"type":           a.Type,
		"blocking":       a.Blocking,
		"message":        a.Message,
		"options":        opts,
		"resume_command": a.ResumeCommand,
	}
	if len(a.Payload) > 0 {
		m["payload"] = a.Payload
	}
	return m
}

// Token derives the opaque identifier for a boundary.
//
// The decision-relevant payload is part of the derivation, so a boundary
// whose terms changed — a different session, a different price — mints a
// different token and a consent given for the old terms stops working.
// That is the whole reason the payload is hashed rather than just
// displayed: consent is to a specific thing, and a token that survived a
// price change would be consent to something the user never saw.
func Token(actionType string, payload map[string]any) (string, error) {
	key, err := secret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(actionType))
	mac.Write([]byte{0})
	mac.Write(canonical(payload))
	return TokenPrefix + hex.EncodeToString(mac.Sum(nil))[:24], nil
}

// Verify reports whether token is the current token for this boundary.
//
// A token for a boundary that has since changed fails here, which is the
// intended outcome: the caller re-runs, gets the new terms and a new token,
// and has to put the new numbers in front of the user.
func Verify(token, actionType string, payload map[string]any) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	want, err := Token(actionType, payload)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
}

// canonical serialises the payload so the same decision always hashes the
// same way regardless of map iteration order.
func canonical(payload map[string]any) []byte {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v, err := json.Marshal(payload[k])
		if err != nil {
			continue
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.Write(v)
		b.WriteByte(';')
	}
	return []byte(b.String())
}

// keyFile is where the per-installation minting key lives. It follows the
// same config-home resolution as everything else, so VIBEKNOW_CONFIG_HOME
// isolates it and tests get their own.
func keyFile() (string, error) {
	d, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "action.key"), nil
}

// secret loads the minting key, creating it on first use.
func secret() ([]byte, error) {
	p, err := keyFile()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err == nil && len(b) >= 32 {
		return b, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	// A racing process may have written its own key between the read above
	// and this write. Either key works — both processes mint and verify
	// against whatever is on disk at the time — so the loser of the race
	// re-reads rather than failing.
	if err := os.WriteFile(p, key, 0o600); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(p); err == nil && len(b) >= 32 {
		return b, nil
	}
	return key, nil
}
