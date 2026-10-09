package notify

import "strings"

// secretShapedKeySubstrings are SPEC §6.4's exact five case-insensitive
// substrings: values whose key matches
// `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are masked everywhere,
// event-hook payloads included. This is the ONE place the list lives; the
// TUI's views call IsSecretShapedKey rather than re-deriving it.
var secretShapedKeySubstrings = []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "CREDENTIAL"}

// MaskedPlaceholder replaces a masked value. It is fixed-width and
// content-free so it never leaks the real value's length.
const MaskedPlaceholder = "********"

// IsSecretShapedKey reports whether key matches SPEC §6.4's secret-shaped
// pattern, case-insensitively, by substring.
func IsSecretShapedKey(key string) bool {
	if isASCII(key) {
		return asciiKeyIsSecretShaped(key)
	}
	upper := strings.ToUpper(key)
	for _, substr := range secretShapedKeySubstrings {
		if strings.Contains(upper, substr) {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiKeyIsSecretShaped is IsSecretShapedKey for an all-ASCII key without
// allocating an upper-cased copy: a key is looked at once per KEY=VALUE pair
// of a free-text reason, which may hold tens of thousands of them.
func asciiKeyIsSecretShaped(key string) bool {
	for _, substr := range secretShapedKeySubstrings {
		for i := 0; i+len(substr) <= len(key); i++ {
			if asciiEqualFoldUpper(key[i:i+len(substr)], substr) {
				return true
			}
		}
	}
	return false
}

// asciiEqualFoldUpper reports whether s upper-cased equals upper, which must
// already be upper case.
func asciiEqualFoldUpper(s, upper string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c != upper[i] {
			return false
		}
	}
	return true
}
