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
	// share is a user-facing page URL served from the production web host, not the beta API cluster.
	"share": "https://vibeknow.com/share",
}
