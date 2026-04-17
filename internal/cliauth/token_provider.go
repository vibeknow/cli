package cliauth

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/credential"
)

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
		if err != nil {
			// Fall back to current token if access token hasn't fully expired yet.
			if st.AccessToken != "" {
				return st.AccessToken, nil
			}
			return "", err
		}
		return newTok, nil
	case credential.StatusExpired:
		// Clean up the dead credential.
		if p.keychainSrc.Keychain != nil && p.keychainSrc.Entry != "" {
			_ = p.keychainSrc.Keychain.Delete(p.keychainSrc.Entry)
		}
		return "", fmt.Errorf("login expired; run `vibeknow auth login`")
	default:
		return st.AccessToken, nil
	}
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
				return "", fmt.Errorf("token refresh failed: %w", err)
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
		return "", err
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
