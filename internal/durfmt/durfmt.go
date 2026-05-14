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
