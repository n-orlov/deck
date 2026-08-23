package tui

import "strings"

// secretShapedKeySubstrings are SPEC §6.4's exact five case-insensitive
// substrings ("Values whose key matches
// `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are masked in every
// view"). This is the ONE place that list lives; every view that renders
// an env-shaped key/value pair (the `e` session env editor, the `,`
// settings takeover's `[env]` entries editor -- the two surfaces that
// actually render one today) calls isSecretShapedKey or maskEnvValue
// rather than re-deriving the pattern.
var secretShapedKeySubstrings = []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "CREDENTIAL"}

// isSecretShapedKey reports whether key matches SPEC §6.4's secret-shaped
// pattern, case-insensitively, by substring (so "AUDIT_ENV_TOKEN",
// "api_key" and "DB_PASSWORD" all match, exactly as "*TOKEN*" etc. read as
// glob patterns would).
func isSecretShapedKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, substr := range secretShapedKeySubstrings {
		if strings.Contains(upper, substr) {
			return true
		}
	}
	return false
}

// maskedSecretPlaceholder is what a masked secret-shaped value renders as,
// deliberately fixed-width and content-free (it must never leak the real
// value's length): the ASCII form uses only §11's plain glyphs.
func (m Model) maskedSecretPlaceholder() string {
	return m.glyph("••••••••", "********")
}

// maskEnvValue is the single place §6.4's "masked in every view ...
// reveal is a per-view explicit toggle" rule is applied to a rendered
// key/value pair: revealed (the view's own reveal toggle is on) or the key
// is not secret-shaped, the real value renders; otherwise the fixed
// placeholder does, never a truncated or partially-shown form of the real
// value.
func (m Model) maskEnvValue(key, value string, revealed bool) string {
	if revealed || !isSecretShapedKey(key) {
		return value
	}
	return m.maskedSecretPlaceholder()
}
