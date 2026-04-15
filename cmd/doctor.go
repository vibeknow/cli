package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/i18n"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

type check struct {
	name string
	fn   func() error
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "diagnose local setup (P0 runs local checks only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(i18n.T("doctor.header"))
		checks := []check{
			{"config directory writable", checkConfigDir},
			{"profiles.yaml parseable", checkProfiles},
			{"keychain backend reachable", checkKeychain},
			{"locale detection", checkLocale},
		}
		failed := 0
		for _, c := range checks {
			if err := c.fn(); err != nil {
				fmt.Println(i18n.T("doctor.fail", c.name, err.Error()))
				failed++
			} else {
				fmt.Println(i18n.T("doctor.ok", c.name))
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(doctorCmd) }

func checkConfigDir() error {
	d, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(d, ".write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(tmp)
}

func checkProfiles() error {
	_, err := config.LoadProfiles()
	return err
}

func checkKeychain() error {
	tmp, err := os.MkdirTemp("", "vibeknow-doctor-probe-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	_, err = keychain.OpenFor("vibeknow-doctor-probe",
		keychain.WithFileBackend(tmp, "probe-passphrase"))
	return err
}

func checkLocale() error {
	if got := i18n.T("doctor.header"); got == "" {
		return fmt.Errorf("i18n returned empty string")
	}
	return nil
}
