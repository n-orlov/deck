package notify

import "strings"

// SPEC §6.4's exact five case-insensitive substrings: values whose key matches
// `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are masked everywhere,
// event-hook payloads included. This is the ONE place the list lives; the
// TUI's views call IsSecretShapedKey rather than re-deriving it.
const (
	secretToken      = "TOKEN"
	secretSecret     = "SECRET"
	secretKey        = "KEY"
	secretPassword   = "PASSWORD"
	secretCredential = "CREDENTIAL"
)

// secretShapedKeySubstrings lists the five constants above, for callers that
// walk the list.
var secretShapedKeySubstrings = []string{secretToken, secretSecret, secretKey, secretPassword, secretCredential}

// MaskedPlaceholder replaces a masked value. It is fixed-width and
// content-free so it never leaks the real value's length.
const MaskedPlaceholder = "********"

// IsSecretShapedKey reports whether key matches SPEC §6.4's secret-shaped
// pattern, case-insensitively, by substring.
//
// It is one expression over stdlib calls on purpose. A free-text reason is
// looked at once per KEY=VALUE pair, and under the nightly lane's -race
// -covermode=atomic every counted Go block (a loop step, an if body) is a
// race-detector atomic: the loop this once was cost more than the whole
// pair's parsing, and a megabyte reason of short pairs missed the detached
// handoff's bound (SPEC §10.3).
func IsSecretShapedKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.Contains(upper, secretToken) || strings.Contains(upper, secretSecret) ||
		strings.Contains(upper, secretKey) || strings.Contains(upper, secretPassword) ||
		strings.Contains(upper, secretCredential)
}
