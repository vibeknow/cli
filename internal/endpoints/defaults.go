// Package endpoints resolves per-service URLs from a profile, with built-in
// cloud defaults. See spec §4.3.
package endpoints

// CloudDefaults lists the built-in production URLs for each service. The
// cluster uses a single host with path-based routing per service; base URLs
// retain no trailing slash so httpclient's `baseURL + path` concatenation
// produces correct URLs when service paths begin with "/v1/...".
var CloudDefaults = map[string]string{
	"account":  "https://vibeknow.com/account",
	"vectoria": "https://vibeknow.com/vectoria",
	"figlens":  "https://vibeknow.com/figlens",
	"vibeknow": "https://vibeknow.com/vibeknow",
	"share":    "https://vibeknow.com/share",
}
