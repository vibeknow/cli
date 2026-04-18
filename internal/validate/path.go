// Package validate contains safe-by-default validators for user-supplied
// inputs — in particular filesystem paths coming from command flags. The
// helpers reject control characters, dangerous Unicode, absolute targets,
// and symlink escapes so malicious or buggy callers cannot cause the CLI to
// overwrite files outside the working directory.
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibeknow/cli/internal/charcheck"
)

// SafeOutputPath validates a download / export target path for --output flags.
// Returns the canonical (symlink-resolved) absolute path, or an error.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// SafeInputPath validates an upload / read source path for --file flags.
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// SafeLocalFlagPath validates a flag value as a local file path. Empty values
// and http(s) URLs are returned unchanged — callers handle those separately.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %v", flagName, err)
	}
	return value, nil
}

// SafeEnvDirPath validates an env-provided application directory. Absolute
// path is required (env vars pointing at relative dirs are almost always
// configuration errors); symlinks through existing ancestors are resolved.
func SafeEnvDirPath(path, envName string) (string, error) {
	if err := charcheck.RejectControlChars(path, envName); err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", envName, path)
	}
	resolved, err := resolveNearestAncestor(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks: %w", err)
	}
	return resolved, nil
}

func safePath(raw, flagName string) (string, error) {
	if err := charcheck.RejectControlChars(raw, flagName); err != nil {
		return "", err
	}
	path := filepath.Clean(raw)
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be a relative path within the current directory, got %q (hint: cd to the target directory first, or use a relative path like ./filename)", flagName, raw)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	resolved := filepath.Join(cwd, path)

	if _, err := os.Lstat(resolved); err == nil {
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	} else {
		resolved, err = resolveNearestAncestor(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	}

	canonicalCwd, _ := filepath.EvalSymlinks(cwd)
	if !isUnderDir(resolved, canonicalCwd) {
		return "", fmt.Errorf("%s %q resolves outside the current working directory (hint: the path must stay within the working directory after resolving .. and symlinks)", flagName, raw)
	}
	return resolved, nil
}

// resolveNearestAncestor walks up the path until it finds an existing
// component, resolves that through EvalSymlinks, and reattaches the
// non-existing tail. This lets us reject symlink escapes even when the final
// target doesn't exist yet (common case: downloading to a new filename).
func resolveNearestAncestor(path string) (string, error) {
	var tail []string
	cur := path
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			parts := append([]string{real}, tail...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			parts := append([]string{cur}, tail...)
			return filepath.Join(parts...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
