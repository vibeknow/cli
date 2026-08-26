package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/keychain"
)

var logoutCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "logout",
	Short: "clear stored credential for the current profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Any authorization still in flight is revoked too. Without this a
		// user who disconnects while the browser page is still open would
		// be silently signed back in by the next `auth status`, which
		// completes parked device codes.
		clearPendingDevice()

		p, err := cliauth.CurrentProfile()
		if err != nil {
			// Logging out of nothing is a success, not an error. A
			// connector host calls this to disconnect and has no way to
			// know whether a profile was ever written; failing here would
			// leave "disconnect" reporting an error for a user who is, by
			// any reading, logged out.
			var noProfile *cliauth.NoActiveProfileError
			var notFound *cliauth.ProfileNotFoundError
			if errors.As(err, &noProfile) || errors.As(err, &notFound) {
				return cmdutil.Emit(cmd, map[string]any{
					"profile":             "",
					"cleared":             false,
					"revoked":             false,
					"env_token_still_set": os.Getenv("VIBEKNOW_TOKEN") != "",
				}, "auth.logout", func(w io.Writer) {
					fmt.Fprintln(w, "no stored credential to clear")
				})
			}
			return err
		}
		// A profile that never completed a login has no credential to clear,
		// which is the state logout exists to produce. Reporting it as an
		// error failed the same requirement the branch above satisfies —
		// disconnect has to stay idempotent and succeed when there is nothing
		// signed in — and a connector host runs exactly this command to
		// disconnect, so an error here surfaces as a failed disconnect for a
		// user who is plainly disconnected.
		if p.CredentialRef == "" {
			return cmdutil.Emit(cmd, map[string]any{
				"profile":             p.Name,
				"cleared":             false,
				"revoked":             false,
				"env_token_still_set": os.Getenv("VIBEKNOW_TOKEN") != "",
			}, "auth.logout", func(w io.Writer) {
				fmt.Fprintln(w, "no stored credential to clear")
			})
		}
		kc, err := keychain.OpenFor("vibeknow")
		if err != nil {
			return err
		}

		// End the session server-side before dropping the local copy — after
		// it, the refresh token needed to do so is gone.
		//
		// Deleting the local credential is what the user asked for and it
		// happens either way. A server that is unreachable, or that no longer
		// recognises this token, must not leave "disconnect" reporting a
		// failure for a user who is by any reading disconnected. The result is
		// reported rather than swallowed: `revoked` tells a caller whether the
		// token is dead everywhere or merely gone from this machine, which is
		// a real difference on a shared or compromised device.
		revoked := revokeSession(cmd, p, kc)

		cleared := kc.Delete(p.CredentialRef) == nil
		if !cleared {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: credential was not present in keychain (may have been env-only)")
		}
		envTokenSet := os.Getenv("VIBEKNOW_TOKEN") != ""
		if envTokenSet {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: VIBEKNOW_TOKEN env var is set and still overrides the keychain — unset it to complete logout")
		}
		// env_token_still_set is in the payload because logout does not
		// actually log you out while it is true, and the caller has no other
		// way to find that out from the exit code.
		return cmdutil.Emit(cmd, map[string]any{
			"profile":             p.Name,
			"credential_ref":      p.CredentialRef,
			"cleared":             cleared,
			"revoked":             revoked,
			"env_token_still_set": envTokenSet,
		}, "auth.logout", func(w io.Writer) {
			if cleared {
				fmt.Fprintf(w, "cleared keychain entry %q\n", p.CredentialRef)
			}
		})
	},
}

// revokeSession asks the account service to end this session, and reports
// whether it did. Best effort by design: see the call site.
//
// A PAT is not a session and has nothing to revoke — it has no refresh token
// and is not something this CLI issued — so it is skipped rather than sent to
// an endpoint that could only reject it.
func revokeSession(cmd *cobra.Command, p config.Profile, kc *keychain.Keychain) bool {
	ks := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
	st, err := ks.GetStored()
	if err != nil || st.RefreshToken == "" || st.TokenType == "pat" {
		return false
	}
	accountURL, err := endpoints.Resolve(p, "account")
	if err != nil {
		return false
	}
	// Bounded tightly: this runs inside `auth logout`, which a connector host
	// expects to return promptly, and the local part of the work does not
	// depend on the answer.
	ctx, cancel := context.WithTimeout(cmd.Context(), revokeTimeout)
	defer cancel()
	if err := account.NewUnauthenticated(accountURL).Logout(ctx, st.RefreshToken); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"note: could not end the session on the server (%v); the local credential was still cleared\n", err)
		return false
	}
	return true
}

// revokeTimeout bounds the server-side revocation attempt during logout.
// Overrunning it is not a failure: the local credential is cleared regardless,
// and a logout that hung on a slow network would be worse than one that gave
// up and said so.
const revokeTimeout = 5 * time.Second
