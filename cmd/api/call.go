package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/credential"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

var callFlags struct {
	service string
	method  string
	path    string
	body    string
}

var callCmd = &cobra.Command{
	Use:   "call",
	Short: "call a raw backend endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if f.Current == "" {
			return fmt.Errorf("no active profile")
		}
		var prof *config.Profile
		for i := range f.Profiles {
			if f.Profiles[i].Name == f.Current {
				prof = &f.Profiles[i]
				break
			}
		}
		if prof == nil {
			return fmt.Errorf("current profile %q not found", f.Current)
		}
		url, err := endpoints.Resolve(*prof, callFlags.service)
		if err != nil {
			return err
		}

		r := credential.Resolver{Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"}}
		if prof.CredentialRef != "" {
			if kc, err := keychain.OpenFor("vibeknow"); err == nil {
				r.Keychain = credential.KeychainSource{Keychain: kc, Entry: prof.CredentialRef}
			}
		}
		tok, _, _ := r.Resolve()

		var bodyReader *bytes.Reader
		if callFlags.body != "" {
			data, err := readBody(callFlags.body)
			if err != nil {
				return err
			}
			bodyReader = bytes.NewReader(data)
		}

		fullURL := strings.TrimRight(url, "/") + callFlags.path
		req, err := http.NewRequestWithContext(context.Background(), callFlags.method, fullURL, nil)
		if err != nil {
			return err
		}
		if bodyReader != nil {
			req.Body = io.NopCloser(bodyReader)
			req.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		chain := httpclient.Chain(http.DefaultTransport,
			httpclient.TraceIDMiddleware{},
			httpclient.VersionMiddleware{Expected: httpclient.ClientAPIVersion},
		)
		resp, err := chain.RoundTrip(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(os.Stdout, resp.Body)
		if resp.StatusCode >= 400 {
			return fmt.Errorf("\nHTTP %d", resp.StatusCode)
		}
		return nil
	},
}

func init() {
	callCmd.Flags().StringVar(&callFlags.service, "service", "", "target service: account|vectoria|figlens|vibeknow (required)")
	callCmd.Flags().StringVar(&callFlags.method, "method", "GET", "HTTP method")
	callCmd.Flags().StringVar(&callFlags.path, "path", "", "URL path including leading slash (required)")
	callCmd.Flags().StringVar(&callFlags.body, "body", "", "JSON body (literal) or @file to read from file")
	_ = callCmd.MarkFlagRequired("service")
	_ = callCmd.MarkFlagRequired("path")
}

func readBody(spec string) ([]byte, error) {
	if strings.HasPrefix(spec, "@") {
		path := strings.TrimPrefix(spec, "@")
		return os.ReadFile(path)
	}
	return []byte(spec), nil
}
