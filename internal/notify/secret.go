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
	upper := strings.ToUpper(key)
	for _, substr := range secretShapedKeySubstrings {
		if strings.Contains(upper, substr) {
			return true
		}
	}
	return false
}
