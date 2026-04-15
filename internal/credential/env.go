package credential

import "os"

// EnvSource reads a token from a named environment variable.
type EnvSource struct{ Var string }

// Get returns the token or ErrNotFound if the env var is empty/unset.
func (e EnvSource) Get() (string, error) {
	v := os.Getenv(e.Var)
	if v == "" {
		return "", ErrNotFound
	}
	return v, nil
}
