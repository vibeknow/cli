// Package durfmt parses durations with a `d` (day) suffix that Go's
// time.ParseDuration doesn't support natively.
package durfmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseAge parses a duration like "7d", "24h", "1h30m".
// "Nd" (a non-empty digit run followed by "d", and nothing else) is
// rewritten to N*24h. Other forms delegate to time.ParseDuration.
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		nStr := strings.TrimSuffix(s, "d")
		n, err := strconv.Atoi(nStr)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid days in %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// Short renders an elapsed duration as a single coarse unit — "3m", "2h",
// "5d". Used for age columns, where the reader wants "recent or not", not
// "2h13m47.9s".
func Short(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
