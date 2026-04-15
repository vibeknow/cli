package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shiliu-ai/vibeknow-cli/internal/redact"
)

// VerboseMiddleware logs request/response summaries to Out. If Out is nil,
// the middleware is a no-op. Credentials in URLs/error messages are redacted
// via the redact package.
type VerboseMiddleware struct {
	Out io.Writer
}

func (m VerboseMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	if m.Out == nil {
		return next
	}
	out := m.Out
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
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
