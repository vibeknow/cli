// Package charcheck strips control characters and escape sequences from
// untrusted text before it is written to a terminal. See spec §8.5.
package charcheck

import (
	"strings"
	"unicode/utf8"
)

// Strip removes C0 (except \t \n), C1, and ANSI escape sequences (CSI + OSC)
// from s. Valid UTF-8 above the control ranges is preserved.
func Strip(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		// If the rune is RuneError with size 1, it's an invalid/non-UTF-8 byte.
		// Treat the raw byte value directly for control-range checks.
		if r == utf8.RuneError && size == 1 {
			bb := s[i]
			if bb >= 0x7f && bb <= 0x9f {
				// drop C1 / DEL byte
				i++
				continue
			}
			// other invalid bytes: drop silently
			i++
			continue
		}
		switch {
		case r == 0x1b && i+1 < len(s) && s[i+1] == '[':
			// CSI: ESC [ ... final byte in 0x40..0x7E
			j := i + 2
			for j < len(s) {
				bb := s[j]
				j++
				if bb >= 0x40 && bb <= 0x7e {
					break
				}
			}
			i = j
			continue
		case r == 0x1b && i+1 < len(s) && s[i+1] == ']':
			// OSC: ESC ] ... BEL or ST
			j := i + 2
			for j < len(s) && s[j] != 0x07 {
				j++
			}
			if j < len(s) {
				j++ // consume BEL
			}
			i = j
			continue
		case r == '\t' || r == '\n':
			b.WriteByte(byte(r))
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
			// drop
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}
