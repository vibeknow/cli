package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/tui"
	"golang.org/x/term"
)

var initCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "init",
	Short: "set up VibeKnow CLI (login + profile)",
	Long:  "Interactive setup wizard: creates a default profile and logs you in.",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	if os.Getenv("VIBEKNOW_TOKEN") != "" {
		fmt.Fprintln(os.Stderr, "warning: VIBEKNOW_TOKEN environment variable is set; it will take priority over stored credentials")
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("init requires an interactive terminal; use `vibeknow auth login --with-token` for non-interactive setup")
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, i18n.T("init.welcome"))
	fmt.Fprintln(os.Stderr, i18n.T("init.welcome.sub"))
	fmt.Fprintln(os.Stderr, "")

	// Step 1: Check existing setup
	p, err := cliauth.CurrentProfile()
	if err == nil {
		// Profile exists, check if logged in
		tok, _, resolveErr := cliauth.ResolverFor(p).Resolve()
		if resolveErr == nil && tok != "" {
			accountURL, _ := endpoints.Resolve(p, "account")
			ac := account.New(accountURL, httpclient.StaticToken(tok))
			if u, whoamiErr := ac.Whoami(context.Background()); whoamiErr == nil {
				display := u.Nickname
				if u.Email != "" {
					display = fmt.Sprintf("%s (%s)", u.Nickname, u.Email)
				}
				fmt.Fprintln(os.Stderr, i18n.T("init.logged_in_as", display))
				fmt.Fprintln(os.Stderr, i18n.T("init.profile", p.Name))
				showBalance(p, tok)
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, i18n.T("init.start_hint"))
				fmt.Fprintln(os.Stderr, "")
				return nil
			}
		}
	}

	// Step 2: Ensure default profile
	p, _, profileErr := ensureDefaultProfile()
	if profileErr != nil {
		return profileErr
	}
	fmt.Fprintln(os.Stderr, i18n.T("init.profile", p.Name))

	// Step 3: Login
	fmt.Fprintln(os.Stderr, "")

	var loginMethod string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T("init.login.prompt")).
				Options(
					huh.NewOption(i18n.T("init.login.opt.browser"), "browser"),
					huh.NewOption(i18n.T("init.login.opt.token"), "token"),
				).
				Value(&loginMethod),
		),
	).WithTheme(tui.ThemeVibeKnow())

	if formErr := form.Run(); formErr != nil {
		if formErr == huh.ErrUserAborted {
			return nil
		}
		return formErr
	}

	// Delegate to existing login logic via cobra
	switch loginMethod {
	case "browser":
		loginArgs := []string{"auth", "login"}
		rootCmd.SetArgs(loginArgs)
		if loginErr := rootCmd.Execute(); loginErr != nil {
			return loginErr
		}
	case "token":
		loginArgs := []string{"auth", "login", "--with-token"}
		rootCmd.SetArgs(loginArgs)
		if loginErr := rootCmd.Execute(); loginErr != nil {
			return loginErr
		}
	}

	// Step 4: Show balance
	p, _ = cliauth.CurrentProfile()
	tok, _, _ := cliauth.ResolverFor(p).Resolve()
	if tok != "" {
		showBalance(p, tok)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, i18n.T("init.start_hint"))
	fmt.Fprintln(os.Stderr, "")
	return nil
}

func ensureDefaultProfile() (config.Profile, string, error) {
	p, err := cliauth.CurrentProfile()
	if err == nil {
		accountURL, _ := endpoints.Resolve(p, "account")
		return p, accountURL, nil
	}

	p = config.Profile{
		Name:          "default",
		CredentialRef: "vibeknow.default",
		Endpoints:     endpoints.CloudDefaults,
		Trust:         "user",
		IsProduction:  true,
	}

	f, loadErr := config.LoadProfiles()
	if loadErr != nil {
		return p, "", loadErr
	}

	found := false
	for _, ep := range f.Profiles {
		if ep.Name == p.Name {
			found = true
			break
		}
	}
	if !found {
		if addErr := config.AddProfile(p); addErr != nil {
			return p, "", addErr
		}
		_ = config.UseProfile(p.Name)
	}

	accountURL, _ := endpoints.Resolve(p, "account")
	return p, accountURL, nil
}

func showBalance(p config.Profile, tok string) {
	vkURL, err := endpoints.Resolve(p, "vibeknow")
	if err != nil {
		return
	}
	vc := vibeknow.New(vkURL, httpclient.StaticToken(tok))
	b, err := vc.GetBalance(context.Background())
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stderr, i18n.T("init.credits.available", b.TotalBalance))
}
