package notify

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// minScrubLen is the shortest session env value scrubbed by value. A one-
// to three-character value ("1", "on") would shred any message that merely
// contains the characters, and leaks nothing worth protecting; the
// KEY=VALUE masking still covers a secret-shaped pair of any length.
const minScrubLen = 4

// assignment finds KEY=VALUE and KEY: VALUE pairs in free text, where VALUE
// is a quoted string or a run of non-space characters.
var assignment = regexp.MustCompile(`([A-Za-z0-9_.\-]+)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)

// Redact applies SPEC §6.4 to free text: every value of a secret-shaped
// KEY=VALUE pair in the text is masked, and every session env value is
// removed wherever it appears. The result never contains a session env
// value that the rules above cover.
func Redact(text string, sessionEnv map[string]string) string {
	text = assignment.ReplaceAllStringFunc(text, maskAssignment)
	for _, value := range scrubValues(sessionEnv) {
		text = strings.ReplaceAll(text, value, MaskedPlaceholder)
	}
	return text
}

// maskAssignment masks the value half of one matched KEY=VALUE pair when its
// key is secret-shaped and leaves every other pair alone.
func maskAssignment(match string) string {
	parts := assignment.FindStringSubmatch(match)
	if !IsSecretShapedKey(parts[1]) {
		return match
	}
	return parts[1] + parts[2] + MaskedPlaceholder
}

// scrubValues lists the session env values to remove by value, longest first
// so a value that contains another is removed whole.
func scrubValues(sessionEnv map[string]string) []string {
	values := make([]string, 0, len(sessionEnv))
	for _, value := range sessionEnv {
		if len(value) < minScrubLen {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

// truncate caps text at limit bytes on a rune boundary and marks the cut.
func truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}
