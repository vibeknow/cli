package cliauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/httpclient"
)

// CodeSessionExpired is the CLI-facing error code returned when the stored
// credential has been permanently invalidated by the backend (session replaced
// on another device, account disabled, or account pending deletion). The
// credential has already been purged from the keychain when this is returned;
// the user must run `vibeknow auth login` to continue.
const CodeSessionExpired = "session_expired"

// sessionDeadCodes are the error codes that mean "this refresh token will
// never work again". Includes the raw backend codes seen in refresh
// responses and the CodeSessionExpired wrapper emitted by doRefresh after
// it has already purged the keychain, so callers up-stack can recognize
// the same condition regardless of which layer observed it first.
//
// CodeAuthRequired — a plain 401 — belongs here because both callers of
// isSessionDead sit on the refresh path, where a 401 is the server saying
// it will not accept this refresh token: expired, revoked, or signed with a
// key that has since been rotated. Treating it as transient leaves a dead
// credential in the keychain and turns every later command into the same
// opaque failure, when the honest answer is "log in again".
var sessionDeadCodes = map[string]struct{}{
	httpclient.CodeAuthRequired:           {},
	httpclient.CodeSessionReplaced:        {},
	httpclient.CodeAccountDisabled:        {},
	httpclient.CodeAccountPendingDeletion: {},
	CodeSessionExpired:                    {},
}

// IsSessionDead reports whether err indicates the credential that produced it
// is permanently invalid — the server will not accept it and no refresh can
// revive it — as opposed to a transient network or server error.
//
// Exported for `auth status`, which has to tell the two apart: a rejected
// credential means "disconnected, log in again", while an unreachable server
// means "unknown, keep the last known state". Answering the second case the
// way we answer the first would flap a healthy connection every time the
// network hiccuped.
func IsSessionDead(err error) bool { return isSessionDead(err) }

// isSessionDead reports whether err indicates the stored credential is
// permanently invalid (as opposed to a transient network or server error).
func isSessionDead(err error) bool {
	for code := range sessionDeadCodes {
		if errs.HasCode(err, code) {
			return true
		}
	}
	return false
}

// OAuthTokenProvider implements httpclient.RefreshableTokenProvider.
// It reads tokens from the keychain, handles three-level status (valid /
// needs_refresh / expired), and performs automatic refresh with cross-process
// locking.
type OAuthTokenProvider struct {
	keychainSrc credential.KeychainSource
	accountURL  string
	lockDir     string
	mu          sync.Mutex
	cachedToken *credential.StoredToken // in-memory fallback if keychain write fails
}

// NewOAuthTokenProvider creates a provider that reads/writes the given keychain
// entry and refreshes against the account service at accountURL.
func NewOAuthTokenProvider(keychainSrc credential.KeychainSource, accountURL, lockDir string) *OAuthTokenProvider {
	return &OAuthTokenProvider{
		keychainSrc: keychainSrc,
		accountURL:  accountURL,
		lockDir:     lockDir,
	}
}

// Token returns the current access token, refreshing if necessary.
func (p *OAuthTokenProvider) Token(ctx context.Context) (string, error) {
	st, err := p.loadToken()
	if err != nil {
		return "", fmt.Errorf("no credential found; run `vibeknow auth login`")
	}

	switch st.Status() {
	case credential.StatusValid:
		return st.AccessToken, nil
	case credential.StatusNeedsRefresh:
		newTok, err := p.doRefresh(ctx, st)
		if err == nil {
			return newTok, nil
		}
		// Session permanently dead — doRefresh has already purged the
		// keychain; surface the re-login prompt directly.
		if isSessionDead(err) {
			return "", err
		}
		// Transient refresh failure (network, 5xx, etc.). The stored
		// access token still has up to 5 minutes of validity (that is
		// why Status() returned NeedsRefresh, not Expired), so let the
		// caller try it. If it also fails with 401, mw_refresh_retry
		// will force one more refresh before giving up.
		return st.AccessToken, nil
	case credential.StatusExpired:
		p.purgeCredential()
		return "", fmt.Errorf("login expired; run `vibeknow auth login`")
	}
	return st.AccessToken, nil
}

// TokenType returns the token_type ("oauth" or "pat") of the stored credential.
func (p *OAuthTokenProvider) TokenType() string {
	st, err := p.loadToken()
	if err != nil {
		return ""
	}
	return st.TokenType
}

// ForceRefresh forces a token refresh regardless of status.
// Returns an error for PAT tokens since they cannot be refreshed.
func (p *OAuthTokenProvider) ForceRefresh(ctx context.Context) (string, error) {
	st, err := p.loadToken()
	if err != nil {
		return "", fmt.Errorf("no credential found; run `vibeknow auth login`")
	}
	if st.TokenType == "pat" {
		return "", fmt.Errorf("PAT tokens cannot be refreshed")
	}
	return p.doRefresh(ctx, st)
}

// loadToken reads the current token, reconciling the keychain against the
// in-memory copy doRefresh keeps when it cannot write one back.
//
// Preferring the keychain whenever it merely *reads* was wrong. A refresh that
// could not be persisted leaves the keychain holding a token the server has
// already spent, and doRefresh's attempt to delete it fails for the same
// reason the write did — a locked keychain refuses both. The stale copy then
// won every subsequent read, so the next call in this process presented the
// spent refresh token a second time. Under rotation that is not a retry: the
// server reads a second presentation as a stolen credential and revokes the
// whole session, signing the user out with nothing to connect it to.
//
// Expiry decides instead, which is the honest comparison and stays right in
// both directions: our unwritten copy beats the stale one it superseded, and a
// token another process has since written beats ours.
func (p *OAuthTokenProvider) loadToken() (credential.StoredToken, error) {
	st, err := p.keychainSrc.GetStored()

	p.mu.Lock()
	var cached *credential.StoredToken
	if p.cachedToken != nil {
		c := *p.cachedToken
		cached = &c
	}
	p.mu.Unlock()

	if err != nil {
		if cached != nil {
			return *cached, nil
		}
		return credential.StoredToken{}, err
	}
	if cached != nil && cached.ExpiresAt.After(st.ExpiresAt) {
		return *cached, nil
	}
	return st, nil
}

// doRefresh performs the refresh using cross-process locking and double-check.
// If the backend reports a permanently-dead session (session replaced on
// another device, account disabled, account pending deletion) it purges the
// stored credential and returns an *errs.Object with code CodeSessionExpired
// so the user sees a single, clear re-login prompt.
func (p *OAuthTokenProvider) doRefresh(ctx context.Context, st credential.StoredToken) (string, error) {
	lock := credential.NewRefreshLock(p.lockDir, p.keychainSrc.Entry)

	tok, err := lock.DoWithDoubleCheck(
		ctx,
		func() bool {
			// Double-check: re-read the keychain and see if another process
			// already refreshed.
			fresh, err := p.keychainSrc.GetStored()
			if err != nil {
				return false
			}
			// If the token changed and is valid, consider it already refreshed.
			return fresh.AccessToken != st.AccessToken && fresh.Status() == credential.StatusValid
		},
		func() (string, error) {
			client := account.NewUnauthenticated(p.accountURL)
			resp, err := client.RefreshToken(ctx, st.RefreshToken)
			if err != nil {
				return "", err
			}

			// A refresh response is allowed to return only a new access
			// token: RFC 6749 §6 makes re-issuing the refresh token
			// optional, and a server that does not rotate simply omits it.
			// Storing the response verbatim then blanked the refresh token
			// and dated the refresh expiry into the past, destroying a
			// working session on its first refresh and forcing a fresh
			// login every access-token lifetime. What the response does not
			// mention, it does not change.
			refreshToken := resp.RefreshToken
			if refreshToken == "" {
				refreshToken = st.RefreshToken
			}
			newST := credential.NewOAuthToken(
				resp.AccessToken,
				refreshToken,
				resp.ExpiresIn,
				resp.RefreshExpiresIn,
			)
			if resp.RefreshExpiresIn == 0 {
				newST.RefreshExpiresAt = st.RefreshExpiresAt
			}

			if err := p.keychainSrc.Keychain.Set(p.keychainSrc.Entry, newST.Marshal()); err != nil {
				// The new pair could not be persisted, so the keychain still
				// holds the old one — and the server has just spent it.
				//
				// Leaving it there is the dangerous option. The next process
				// would read it, present it, and a server that rotates reads a
				// second presentation of a spent token as a stolen credential:
				// it revokes the whole session and logs a security warning.
				// The user would be signed out some minutes later for no
				// reason they could connect to anything they did.
				//
				// So the stale copy is removed instead. It could not have
				// worked again in any case; what changes is that the next
				// process finds no credential and says "log in", rather than
				// tripping an alarm on the way to the same place. The pair
				// stays in memory so the command in flight still finishes.
				log.Printf("warning: failed to write refreshed token to keychain: %v", err)
				p.mu.Lock()
				p.cachedToken = &newST
				p.mu.Unlock()
				if delErr := p.keychainSrc.Keychain.Delete(p.keychainSrc.Entry); delErr != nil {
					log.Printf("warning: could not remove the superseded credential: %v", delErr)
				}
			}

			return newST.AccessToken, nil
		},
	)
	if err != nil {
		if isSessionDead(err) {
			p.purgeCredential()
			return "", newSessionExpired(err)
		}
		return "", fmt.Errorf("token refresh failed: %w", err)
	}

	// If DoWithDoubleCheck returned ("", nil), another process already refreshed.
	// Re-read the keychain for the fresh token.
	if tok == "" {
		fresh, err := p.loadToken()
		if err != nil {
			return "", err
		}
		return fresh.AccessToken, nil
	}

	return tok, nil
}

// newSessionExpired wraps the backend refresh error into the CLI-facing
// session_expired Error Object. The user-facing Message is a short prompt;
// the underlying backend code / message / trace_id are preserved in Details
// so they survive JSON rendering and can be inspected in bug reports.
func newSessionExpired(cause error) *errs.Object {
	out := &errs.Object{
		SchemaVersion: "1",
		Code:          CodeSessionExpired,
		Message:       "session expired; run `vibeknow auth login`",
		Details:       map[string]any{},
	}
	var inner *errs.Object
	if errors.As(cause, &inner) {
		out.TraceID = inner.TraceID
		if inner.Code != "" {
			out.Details["cause_code"] = inner.Code
		}
		if inner.Message != "" {
			out.Details["cause_message"] = inner.Message
		}
		return out
	}
	// Non-structured error (shouldn't normally happen from doRefresh, but
	// defensively keep the raw string).
	out.Details["cause_message"] = cause.Error()
	return out
}

// purgeCredential removes the stored credential from the keychain and clears
// the in-memory fallback cache. Safe to call even if the keychain entry does
// not exist.
func (p *OAuthTokenProvider) purgeCredential() {
	if p.keychainSrc.Keychain != nil && p.keychainSrc.Entry != "" {
		_ = p.keychainSrc.Keychain.Delete(p.keychainSrc.Entry)
	}
	p.mu.Lock()
	p.cachedToken = nil
	p.mu.Unlock()
}
