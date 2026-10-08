package tui

import "github.com/n-orlov/deck/internal/notify"

// isSecretShapedKey reports whether key matches SPEC §6.4's secret-shaped
// pattern. The predicate lives in internal/notify (the event-hook payload
// applies the same rule); every view that renders an env-shaped key/value
// pair (the `e` session env editor, the `,` settings takeover's `[env]`
// entries editor) calls this or maskEnvValue rather than re-deriving it.
func isSecretShapedKey(key string) bool { return notify.IsSecretShapedKey(key) }

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
