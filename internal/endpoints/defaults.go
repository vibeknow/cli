// Package endpoints resolves per-service URLs from a profile, with built-in
// cloud defaults. See spec §4.3.
package endpoints

// CloudDefaults lists the built-in production URLs for each service. The beta
// cluster uses a single host with path-based routing per service; base URLs
// retain no trailing slash so httpclient's `baseURL + path` concatenation
// produces correct URLs when service paths begin with "/v1/...".
var CloudDefaults = map[string]string{
	"account":  "https://beta.lab.shiliu.chat/account",
	"vectoria": "https://beta.lab.shiliu.chat/vectoria",
	"figlens":  "https://beta.lab.shiliu.chat/figlens",
	"vibeknow": "https://beta.lab.shiliu.chat/vibeknow",
	// share is the public page base for the same cluster. Share tokens
	// are cluster-local (beta tokens don't resolve on production), so
	// this must track the rest of the map.
	"share": "https://beta.lab.shiliu.chat/share",
}
