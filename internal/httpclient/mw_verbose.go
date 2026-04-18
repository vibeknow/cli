package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/vibeknow/cli/internal/redact"
)

// VerboseMiddleware logs request/response summaries to Out. If Out is nil,
// the middleware falls back to os.Stderr when VIBEKNOW_DEBUG=1 is set in
// the environment (honored per-request so changes take effect immediately).
// When both Out is nil and VIBEKNOW_DEBUG is unset, the middleware is a
// no-op. Credentials in URLs and error messages are redacted via the
// redact package.
type VerboseMiddleware struct {
	Out io.Writer
}

func (m VerboseMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		out := m.Out
		if out == nil && os.Getenv("VIBEKNOW_DEBUG") == "1" {
			out = os.Stderr
		}
		if out == nil {
			return next.RoundTrip(r)
		}
		start := time.Now()
		resp, err := next.RoundTrip(r)
		dur := time.Since(start)
		line := fmt.Sprintf("%s %s -> ", r.Method, redact.String(r.URL.String()))
		if err != nil {
			line += fmt.Sprintf("err=%s (%s)", redact.String(err.Error()), dur)
		} else {
			line += fmt.Sprintf("%d (%s)", resp.StatusCode, dur)
		}
		fmt.Fprintln(out, line)
		return resp, err
	})
}
