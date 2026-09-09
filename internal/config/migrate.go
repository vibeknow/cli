package config

// frozenDefaultEndpoints holds every URL that endpoints.CloudDefaults has
// ever shipped. Through v0.6.3, `init` and `auth login` copied the running
// binary's built-in defaults into the profile they created, and Resolve
// prefers the profile copy — so a user whose first login happened on a
// binary that defaulted to the beta test cluster kept every later request
// on that cluster, no matter how many upgrades followed. Scrubbing entries
// that exactly equal one of these snapshots returns those profiles to the
// built-in defaults of whatever binary is running; a URL the user chose
// themselves never appears here and is never touched.
//
// The config package cannot see endpoints.CloudDefaults (endpoints depends
// on config), so the current defaults are repeated here;
// endpoints/defaults_test.go asserts the two stay in sync.
var frozenDefaultEndpoints = map[string]bool{
	// Pre-v0.3.1: per-service subdomains.
	"https://account.vibeknow.com":  true,
	"https://vectoria.vibeknow.com": true,
	"https://figlens.vibeknow.com":  true,
	"https://api.vibeknow.com":      true,
	// v0.3.1–v0.6.3: beta test cluster.
	"https://beta.lab.shiliu.chat/account":  true,
	"https://beta.lab.shiliu.chat/vectoria": true,
	"https://beta.lab.shiliu.chat/figlens":  true,
	"https://beta.lab.shiliu.chat/vibeknow": true,
	"https://beta.lab.shiliu.chat/share":    true,
	// v0.7.0+: production.
	"https://vibeknow.com/account":  true,
	"https://vibeknow.com/vectoria": true,
	"https://vibeknow.com/figlens":  true,
	"https://vibeknow.com/vibeknow": true,
	"https://vibeknow.com/share":    true,
}

// IsFrozenDefaultEndpoint reports whether url is a built-in default that an
// old binary may have frozen into a profile. Exported for the consistency
// test in the endpoints package.
func IsFrozenDefaultEndpoint(url string) bool {
	return frozenDefaultEndpoints[url]
}

// scrubFrozenDefaults removes endpoint overrides that are really frozen
// copies of built-in defaults, reporting whether anything changed. Profiles
// with trust=dev and is_production=false are left alone: that combination is
// the sanctioned way to point at a non-production cluster on purpose, and
// Validate only permits non-production URLs under it.
func scrubFrozenDefaults(f *ProfilesFile) bool {
	changed := false
	for i := range f.Profiles {
		p := &f.Profiles[i]
		if p.Trust == "dev" && !p.IsProduction {
			continue
		}
		for svc, u := range p.Endpoints {
			if frozenDefaultEndpoints[u] {
				delete(p.Endpoints, svc)
				changed = true
			}
		}
		// The schema-v1 field feeds Endpoints["vibeknow"] on every load, so
		// a frozen value here would resurrect the entry just scrubbed.
		if frozenDefaultEndpoints[p.APIEndpoint] {
			p.APIEndpoint = ""
			changed = true
		}
	}
	return changed
}
