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
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
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
}

func runLogin(cmd *cobra.Command, args []string) error {
	withToken, _ := cmd.Flags().GetBool("with-token")
	noWait, _ := cmd.Flags().GetBool("no-wait")
	deviceCode, _ := cmd.Flags().GetString("device-code")

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
	if flagCount > 1 {
		return fmt.Errorf("--with-token, --no-wait, and --device-code are mutually exclusive")
	}

	switch {
	case withToken:
		return loginWithToken(cmd)
	case noWait:
		return loginNoWait(cmd)
	case deviceCode != "":
		return loginDeviceCode(cmd, deviceCode)
	default:
		return loginInteractive(cmd)
	}
}

// ---------------------------------------------------------------------------
// Mode 1: Interactive (default)
// ---------------------------------------------------------------------------

func loginInteractive(cmd *cobra.Command) error {
	if !isTerminal() {
		return fmt.Errorf("请使用 --with-token 或 --no-wait 进行非交互式登录")
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
		c := account.New(accountURL, staticToken(tok))
		if u, whoamiErr := c.Whoami(context.Background()); whoamiErr == nil {
			name := u.Nickname
			if name == "" {
				name = u.Email
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "已登录为 %s，是否重新登录？(y/N) ", name)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Fprintln(cmd.ErrOrStderr(), "cancelled")
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

	fmt.Fprintf(cmd.ErrOrStderr(), "\n请在浏览器中输入验证码: %s\n", dcResp.UserCode)
	fmt.Fprintf(cmd.ErrOrStderr(), "验证地址: %s\n\n", dcResp.VerificationURI)
	fmt.Fprint(cmd.ErrOrStderr(), "按 Enter 键打开浏览器...")

	// Wait for Enter.
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')

	// Open browser.
	if err := openBrowser(dcResp.VerificationURI); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "无法打开浏览器: %v\n请手动打开上方链接完成验证。\n", err)
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
		fmt.Fprint(cmd.ErrOrStderr(), "请输入 Personal Access Token: ")
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
	c := account.New(accountURL, staticToken(token))
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
	fmt.Fprintf(cmd.ErrOrStderr(), "✓ 已登录为 %s (PAT)\n", name)
	return nil
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

	output := map[string]interface{}{
		"device_code":      dcResp.DeviceCode,
		"user_code":        dcResp.UserCode,
		"verification_uri": dcResp.VerificationURI,
		"expires_in":       dcResp.ExpiresIn,
		"hint":             fmt.Sprintf("请访问 %s 并输入验证码 %s", dcResp.VerificationURI, dcResp.UserCode),
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(output)
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
			return nil, fmt.Errorf("device code expired; please try again")
		}

		remaining := time.Until(deadline).Truncate(time.Second)
		fmt.Fprintf(cmd.ErrOrStderr(), "\r⏳ 等待授权... 剩余 %s  ", remaining)

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
					return nil, fmt.Errorf("device code expired; please try again")
				case account.PollDenied:
					fmt.Fprintln(cmd.ErrOrStderr())
					return nil, fmt.Errorf("authorization denied by user")
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
	c := account.New(accountURL, staticToken(tokenResp.AccessToken))
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
		fmt.Fprintf(cmd.ErrOrStderr(), "✓ 欢迎, %s!\n", name)
	}
	return nil
}
