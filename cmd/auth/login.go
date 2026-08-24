package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/keychain"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "authenticate with VibeKnow",
	Long: `Authenticate with VibeKnow using the device code flow (default),
a personal access token (--with-token), or a two-phase agent flow
(--no-wait / --device-code).`,
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().Bool("with-token", false, "read PAT from stdin")
	loginCmd.Flags().Bool("no-wait", false, "print device code and exit (Agent use)")
	loginCmd.Flags().String("device-code", "", "resume polling for a device code (Agent use)")
	loginCmd.Flags().Bool("headless", false, "blocking device-code login with no TTY/Enter-key requirement; prints the device-code JSON envelope to stdout, then polls until authorized (host-automation use, e.g. WorkBuddy CLI connectors that spawn `auth`, scrape stdout for a URL, and let the process keep running)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	withToken, _ := cmd.Flags().GetBool("with-token")
	noWait, _ := cmd.Flags().GetBool("no-wait")
	deviceCode, _ := cmd.Flags().GetString("device-code")
	headless, _ := cmd.Flags().GetBool("headless")

	// Mutual exclusion check.
	flagCount := 0
	if withToken {
		flagCount++
	}
	if noWait {
		flagCount++
	}
	if deviceCode != "" {
		flagCount++
	}
	if headless {
		flagCount++
	}
	if flagCount > 1 {
		return fmt.Errorf("--with-token, --no-wait, --device-code, and --headless are mutually exclusive")
	}

	switch {
	case withToken:
		return loginWithToken(cmd)
	case noWait:
		return loginNoWait(cmd)
	case deviceCode != "":
		return loginDeviceCode(cmd, deviceCode)
	case headless:
		return loginHeadless(cmd)
	default:
		return loginInteractive(cmd)
	}
}

// ---------------------------------------------------------------------------
// Mode 1: Interactive (default)
// ---------------------------------------------------------------------------

func loginInteractive(cmd *cobra.Command) error {
	if !isTerminal() {
		return fmt.Errorf("%s", i18n.T("auth.login.tty_required"))
	}

	// Warn if env var is set.
	if os.Getenv("VIBEKNOW_TOKEN") != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: VIBEKNOW_TOKEN environment variable is set; it will take priority over stored credentials")
	}

	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	// Check if already logged in.
	r := cliauth.ResolverFor(p)
	if tok, _, resolveErr := r.Resolve(); resolveErr == nil {
		c := account.New(accountURL, httpclient.StaticToken(tok))
		if u, whoamiErr := c.Whoami(context.Background()); whoamiErr == nil {
			name := u.Nickname
			if name == "" {
				name = u.Email
			}
			fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.already_prompt", name))
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("auth.login.cancelled"))
				return nil
			}
		}
	}

	// Initiate device code flow.
	unauthClient := account.NewUnauthenticated(accountURL)
	dcResp, err := unauthClient.DeviceCode(context.Background())
	if err != nil {
		return fmt.Errorf("device code request failed: %w", err)
	}

	fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.enter_code", dcResp.UserCode))
	fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.verify_uri", dcResp.VerificationURI))
	fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.press_enter"))

	// Wait for Enter.
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')

	// Open browser.
	if err := openBrowser(dcResp.VerificationURI); err != nil {
		fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.browser_failed", err))
	}

	// Poll for token.
	tokenResp, err := pollDeviceToken(cmd, unauthClient, dcResp.DeviceCode, dcResp.Interval, dcResp.ExpiresIn)
	if err != nil {
		return err
	}

	return finishLogin(cmd, p, accountURL, tokenResp, false)
}

// ---------------------------------------------------------------------------
// Mode 2: --with-token
// ---------------------------------------------------------------------------

func loginWithToken(cmd *cobra.Command) error {
	var token string

	if isTerminal() {
		fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.token_prompt"))
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr()) // newline after hidden input
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			token = strings.TrimSpace(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read token from stdin: %w", err)
		}
	}

	if token == "" {
		return fmt.Errorf("token is empty")
	}

	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	// Verify token via whoami.
	c := account.New(accountURL, httpclient.StaticToken(token))
	u, err := c.Whoami(context.Background())
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}

	st := credential.NewPATToken(token)
	if err := storeToken(p, st); err != nil {
		return err
	}

	name := u.Nickname
	if name == "" {
		name = u.Email
	}
	fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.signed_in_pat", name))
	return emitLoginResult(cmd, p, u, "pat")
}

// ---------------------------------------------------------------------------
// Mode 3: --no-wait
// ---------------------------------------------------------------------------

func loginNoWait(cmd *cobra.Command) error {
	_, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	unauthClient := account.NewUnauthenticated(accountURL)
	dcResp, err := unauthClient.DeviceCode(context.Background())
	if err != nil {
		return fmt.Errorf("device code request failed: %w", err)
	}

	return writeDeviceCodeEnvelope(cmd, dcResp)
}

func writeDeviceCodeEnvelope(cmd *cobra.Command, dcResp *account.DeviceCodeResponse) error {
	output := map[string]any{
		"device_code":      dcResp.DeviceCode,
		"user_code":        dcResp.UserCode,
		"verification_uri": dcResp.VerificationURI,
		"expires_in":       dcResp.ExpiresIn,
		"hint":             i18n.T("auth.login.hint.visit_code", dcResp.VerificationURI, dcResp.UserCode),
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// ---------------------------------------------------------------------------
// Mode 5: --headless
// ---------------------------------------------------------------------------

// loginHeadless is loginNoWait and loginDeviceCode fused into one blocking
// call: it requires no TTY, waits for no Enter keypress, and never tries to
// open a browser itself (a spawning host is expected to do that after
// scraping the verification_uri/user_code JSON fields from stdout — this is
// the shape WorkBuddy's `cli.json` "authDeviceFlow" auth model wants: a
// single `auth` command that prints the verification URL to stdout and then
// keeps running, polling the token endpoint on its own, rather than being
// killed once the URL is found).
func loginHeadless(cmd *cobra.Command) error {
	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	unauthClient := account.NewUnauthenticated(accountURL)
	dcResp, err := unauthClient.DeviceCode(context.Background())
	if err != nil {
		return fmt.Errorf("device code request failed: %w", err)
	}

	if err := writeDeviceCodeEnvelope(cmd, dcResp); err != nil {
		return err
	}

	tokenResp, err := pollDeviceToken(cmd, unauthClient, dcResp.DeviceCode, dcResp.Interval, dcResp.ExpiresIn)
	if err != nil {
		return err
	}

	return finishLogin(cmd, p, accountURL, tokenResp, true)
}

// ---------------------------------------------------------------------------
// Mode 4: --device-code <code>
// ---------------------------------------------------------------------------

func loginDeviceCode(cmd *cobra.Command, deviceCode string) error {
	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	unauthClient := account.NewUnauthenticated(accountURL)

	// Use default interval and a generous expiry for resumed polling.
	tokenResp, err := pollDeviceToken(cmd, unauthClient, deviceCode, 5, 900)
	if err != nil {
		return err
	}

	quiet := !isTerminal()
	return finishLogin(cmd, p, accountURL, tokenResp, quiet)
}

// ---------------------------------------------------------------------------
// Helper: resolveAccountURL
// ---------------------------------------------------------------------------

func resolveAccountURL() (config.Profile, string, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		var noProfile *cliauth.NoActiveProfileError
		var notFound *cliauth.ProfileNotFoundError
		if errors.As(err, &noProfile) || errors.As(err, &notFound) {
			p = config.Profile{
				Name:          "default",
				CredentialRef: "vibeknow.default",
				Endpoints:     endpoints.CloudDefaults,
				Trust:         "user",
				IsProduction:  true,
			}
		} else {
			return config.Profile{}, "", err
		}
	}
	url, err := endpoints.Resolve(p, "account")
	if err != nil {
		return config.Profile{}, "", err
	}
	return p, url, nil
}

// ---------------------------------------------------------------------------
// Helper: storeToken
// ---------------------------------------------------------------------------

func storeToken(p config.Profile, st credential.StoredToken) error {
	// Ensure profile exists.
	f, err := config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}

	found := false
	for _, existing := range f.Profiles {
		if existing.Name == p.Name {
			found = true
			break
		}
	}
	if !found {
		if err := config.AddProfile(p); err != nil {
			return fmt.Errorf("add profile: %w", err)
		}
	}
	// Set as current profile.
	if f.Current != p.Name {
		if err := config.UseProfile(p.Name); err != nil {
			return fmt.Errorf("set current profile: %w", err)
		}
	}

	// Write token to keychain.
	kc, err := keychain.OpenFor("vibeknow")
	if err != nil {
		return fmt.Errorf("open keychain: %w", err)
	}
	if err := kc.Set(p.CredentialRef, st.Marshal()); err != nil {
		return fmt.Errorf("store token in keychain: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helper: pollDeviceToken
// ---------------------------------------------------------------------------

func pollDeviceToken(cmd *cobra.Command, client *account.Client, deviceCode string, interval, expiresIn int) (*account.DeviceTokenResponse, error) {
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	pollInterval := time.Duration(interval) * time.Second

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s", i18n.T("auth.login.code_expired"))
		}

		remaining := time.Until(deadline).Truncate(time.Second)
		fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.waiting", remaining))

		time.Sleep(pollInterval)

		resp, err := client.DeviceToken(context.Background(), deviceCode)
		if err != nil {
			var pollErr *account.PollError
			if errors.As(err, &pollErr) {
				switch pollErr.Status {
				case account.PollPending:
					continue
				case account.PollSlowDown:
					pollInterval += 5 * time.Second
					continue
				case account.PollExpired:
					fmt.Fprintln(cmd.ErrOrStderr())
					return nil, fmt.Errorf("%s", i18n.T("auth.login.code_expired"))
				case account.PollDenied:
					fmt.Fprintln(cmd.ErrOrStderr())
					return nil, fmt.Errorf("%s", i18n.T("auth.login.denied"))
				}
			}
			return nil, fmt.Errorf("poll device token: %w", err)
		}

		fmt.Fprintln(cmd.ErrOrStderr()) // clear spinner line
		return resp, nil
	}
}

// ---------------------------------------------------------------------------
// Helper: openBrowser
// ---------------------------------------------------------------------------

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

// ---------------------------------------------------------------------------
// Helper: isTerminal
// ---------------------------------------------------------------------------

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ---------------------------------------------------------------------------
// Helper: finishLogin
// ---------------------------------------------------------------------------

func finishLogin(cmd *cobra.Command, profile config.Profile, accountURL string, tokenResp *account.DeviceTokenResponse, quiet bool) error {
	// Verify token via whoami.
	c := account.New(accountURL, httpclient.StaticToken(tokenResp.AccessToken))
	u, err := c.Whoami(context.Background())
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}

	st := credential.NewOAuthToken(
		tokenResp.AccessToken,
		tokenResp.RefreshToken,
		tokenResp.ExpiresIn,
		tokenResp.RefreshExpiresIn,
	)
	if err := storeToken(profile, st); err != nil {
		return err
	}

	if !quiet {
		name := u.Nickname
		if name == "" {
			name = u.Email
		}
		fmt.Fprint(cmd.ErrOrStderr(), i18n.T("auth.login.welcome", name))
	}
	return emitLoginResult(cmd, profile, u, "oauth")
}

// emitLoginResult writes the signed-in identity to stdout for structured
// callers. The text form stays exactly as it was — a localized greeting on
// stderr and an empty stdout — because scripts already rely on that; only
// json/ndjson gain a payload, which is what a caller chaining `auth login`
// into `create` actually needs.
func emitLoginResult(cmd *cobra.Command, profile config.Profile, u *account.User, tokenType string) error {
	return cmdutil.Emit(cmd, map[string]any{
		"uid":        u.UID,
		"nickname":   u.Nickname,
		"email":      u.Email,
		"profile":    profile.Name,
		"token_type": tokenType,
	}, "auth.login", nil)
}
