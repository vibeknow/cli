// Package redact masks sensitive values in strings before logging.
// See spec §8.5.
package redact

import "regexp"

var (
	bearerRe  = regexp.MustCompile(`(?i)(authorization:\s*bearer)\s+\S+`)
	basicRe   = regexp.MustCompile(`(?i)(basic)\s+[A-Za-z0-9+/=]+`)
	apiKeyRe  = regexp.MustCompile(`(?i)(x-[a-z-]*api[a-z-]*key|x-auth-token):\s*\S+`)
	sessionRe = regexp.MustCompile(`(?i)(session|sid|token)=[^;,\s]+`)
)

// String returns s with common credential patterns replaced by "***".
func String(s string) string {
	s = bearerRe.ReplaceAllString(s, "$1 ***")
	s = basicRe.ReplaceAllString(s, "$1 ***")
	s = apiKeyRe.ReplaceAllString(s, "$1: ***")
	s = sessionRe.ReplaceAllString(s, "$1=***")
	return s
}
