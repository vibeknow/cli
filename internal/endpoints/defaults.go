// Package endpoints resolves per-service URLs from a profile, with built-in
// cloud defaults. See spec §4.3.
package endpoints

// CloudDefaults lists the built-in production URLs for each service.
// Values are placeholders until ops confirms real domains (spec §10).
var CloudDefaults = map[string]string{
	"account":  "https://account.vibeknow.com",
	"vectoria": "https://vectoria.vibeknow.com",
	"figlens":  "https://figlens.vibeknow.com",
	"vibeknow": "https://api.vibeknow.com",
}
