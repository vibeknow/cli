package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
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
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, callFlags.service)
		if err != nil {
			return err
		}

		// Build a TokenProvider from the resolver.
		tp := resolverTokenProvider{res: cliauth.ResolverFor(p)}

		// Body
		var body any
		if callFlags.body != "" {
			raw, err := readBody(callFlags.body)
			if err != nil {
				return err
			}
			// Pass raw JSON bytes through as-is. httpclient.Client marshals
			// via encoding/json; to avoid double-encoding we use json.RawMessage.
			body = json.RawMessage(raw)
		}

		client := httpclient.New(url).WithTransport(httpclient.StandardChain(tp, nil))

		// Raw layer: print response body to stdout. We do this ourselves
		// rather than decoding into a struct, so use a custom path via
		// http.NewRequestWithContext since httpclient.Client.Do decodes JSON.
		// For symmetry with the rest of the stack, do it manually but still
		// invoke the same chain's transport.
		var reader io.Reader
		if body != nil {
			buf, _ := json.Marshal(body)
			reader = bytes.NewReader(buf)
		}
		fullURL := strings.TrimRight(url, "/") + callFlags.path
		req, err := http.NewRequestWithContext(context.Background(), callFlags.method, fullURL, reader)
		if err != nil {
			return err
		}
		if reader != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		// Extract the transport from the client so we share the middleware stack.
		transport := client.Transport()
		resp, err := transport.RoundTrip(req)
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

// resolverTokenProvider adapts credential.Resolver to httpclient.TokenProvider.
type resolverTokenProvider struct {
	res interface{ Resolve() (string, string, error) }
}

func (r resolverTokenProvider) Token(ctx context.Context) (string, error) {
	tok, _, err := r.res.Resolve()
	if err != nil {
		return "", nil // empty token → AuthMiddleware skips
	}
	return tok, nil
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
