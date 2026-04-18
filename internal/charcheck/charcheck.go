// Package charcheck provides character-level safety primitives shared by two
// concerns:
//   - Strip: sanitize untrusted text before writing to a terminal (spec §8.5).
//   - RejectControlChars / IsDangerousUnicode: reject user-supplied flag
//     values that contain control characters or visual-spoofing runes, used
//     by internal/validate for path/URL inputs.
package charcheck

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// RejectControlChars rejects C0 control characters (except \t, \n) and
// dangerous Unicode runes (Bidi overrides, zero-width, line/paragraph
// separators) that enable visual-spoofing attacks.
func RejectControlChars(value, flagName string) error {
	for _, r := range value {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("%s contains invalid control characters", flagName)
		}
		if IsDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// IsDangerousUnicode identifies runes used to visually spoof filenames or
// inject hidden content (e.g. "report.exe" displayed as "report.txt" via Bidi
// override).
func IsDangerousUnicode(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200D: // zero-width space / non-joiner / joiner
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0x202A && r <= 0x202E: // Bidi: LRE / RLE / PDF / LRO / RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // line / paragraph separator
		return true
	case r >= 0x2066 && r <= 0x2069: // Bidi isolates: LRI / RLI / FSI / PDI
		return true
	}
	return false
}

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
