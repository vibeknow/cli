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
var sessionDeadCodes = map[string]struct{}{
	httpclient.CodeSessionReplaced:        {},
	httpclient.CodeAccountDisabled:        {},
	httpclient.CodeAccountPendingDeletion: {},
	CodeSessionExpired:                    {},
}

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

// loadToken reads the token from the keychain, falling back to the in-memory
// cache if the keychain read fails.
func (p *OAuthTokenProvider) loadToken() (credential.StoredToken, error) {
	st, err := p.keychainSrc.GetStored()
	if err == nil {
		return st, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cachedToken != nil {
		return *p.cachedToken, nil
	}
	return credential.StoredToken{}, err
}

// doRefresh performs the refresh using cross-process locking and double-check.
// If the backend reports a permanently-dead session (session replaced on
// another device, account disabled, account pending deletion) it purges the
// stored credential and returns an *errs.Object with code CodeSessionExpired
// so the user sees a single, clear re-login prompt.
func (p *OAuthTokenProvider) doRefresh(ctx context.Context, st credential.StoredToken) (string, error) {
	lock := credential.NewRefreshLock(p.lockDir, p.keychainSrc.Entry)

	tok, err := lock.DoWithDoubleCheck(
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

			newST := credential.NewOAuthToken(
				resp.AccessToken,
				resp.RefreshToken,
				resp.ExpiresIn,
				resp.RefreshExpiresIn,
			)

			// Write to keychain.
			if err := p.keychainSrc.Keychain.Set(p.keychainSrc.Entry, newST.Marshal()); err != nil {
				// Keychain write failed — cache in memory and log warning.
				log.Printf("warning: failed to write refreshed token to keychain: %v", err)
				p.mu.Lock()
				p.cachedToken = &newST
				p.mu.Unlock()
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
