package agent

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxHarnessNameRunes bounds an adopted harness title (SPEC §8 "Adopting a
// harness name"): the longest name any supported harness accepts for itself.
const MaxHarnessNameRunes = 200

// Per-harness limits on a name deck pushes into a running harness.
const (
	copilotNameMaxRunes = 100
	claudeNameMaxRunes  = 200
)

// namePrefixes are the decorations a harness puts in front of its title: the
// Claude Code activity glyph, its spinner frames and Pi's `π - ` product
// prefix. They are display chrome, never part of the name.
var namePrefixes = []string{"✳", "✶", "✻", "✽", "✢", "·", "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "π -", "π–", "π—"}

// SanitiseHarnessName turns a title a harness reported into a name deck may
// store, or "" when nothing usable is left (SPEC §8). The title is untrusted
// input: terminal escape sequences and C0/C1 control characters are removed,
// whitespace runs collapse to one space, a leading glyph prefix is stripped
// and the result is cut to MaxHarnessNameRunes. "" means no adoption.
func SanitiseHarnessName(raw string) string {
	cleaned := collapseSpace(stripControl(stripEscapes(raw)))
	cleaned = stripNamePrefix(cleaned)
	return truncateRunes(cleaned, MaxHarnessNameRunes)
}

// stripEscapes removes CSI, OSC/DCS/APC/PM/SOS strings and two-byte ESC
// sequences, whole, so no parameter text is left behind as part of the name.
func stripEscapes(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			out.WriteByte(s[i])
			i++
			continue
		}
		i = skipEscape(s, i)
	}
	return out.String()
}

// skipEscape returns the index after the escape sequence that starts at s[i].
func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return i + 1
	}
	switch s[i+1] {
	case '[':
		return skipCSI(s, i+2)
	case ']', 'P', 'X', '^', '_':
		return skipString(s, i+2)
	}
	return i + 2
}

// skipCSI skips parameter and intermediate bytes up to and including the final byte.
func skipCSI(s string, i int) int {
	for i < len(s) {
		c := s[i]
		i++
		if c >= 0x40 && c <= 0x7e {
			break
		}
	}
	return i
}

// skipString skips a string-terminated sequence (OSC and friends) to BEL or ST.
func skipString(s string, i int) int {
	for i < len(s) {
		if s[i] == 0x07 {
			return i + 1
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return i
}

// stripControl drops C0, DEL and C1 controls and invalid UTF-8, turning the
// whitespace controls (tab, newline, CR) into a space first.
func stripControl(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			out.WriteByte(' ')
		case r == utf8.RuneError, unicode.IsControl(r), isFormatOverride(r):
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// isFormatOverride is a bidi control, zero-width or other invisible format
// character, which can disguise a name in a list the user reads: U+200B-200F,
// U+202A-202E, U+2060-2064 (word joiner and invisible operators), U+2066-2069,
// U+00AD (soft hyphen), U+061C (Arabic letter mark), U+180E (Mongolian vowel
// separator), U+FEFF and the tag block U+E0000-E007F.
func isFormatOverride(r rune) bool {
	return (r >= 0x200b && r <= 0x200f) || (r >= 0x202a && r <= 0x202e) ||
		(r >= 0x2060 && r <= 0x2064) || (r >= 0x2066 && r <= 0x2069) ||
		r == 0xfeff || r == 0x00ad || r == 0x061c || r == 0x180e ||
		(r >= 0xe0000 && r <= 0xe007f)
}

// NameComparisonForm is a name as the collision check compares it: escapes,
// controls and invisible format characters removed and whitespace collapsed,
// but neither prefix-stripped nor truncated, so a "-2" suffix always changes it.
func NameComparisonForm(name string) string {
	return collapseSpace(stripControl(stripEscapes(name)))
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// stripNamePrefix removes leading glyph-prefix decorations, repeatedly.
func stripNamePrefix(s string) string {
	for {
		trimmed := s
		for _, prefix := range namePrefixes {
			if rest, ok := strings.CutPrefix(s, prefix); ok && prefixEnds(rest) {
				trimmed = strings.TrimSpace(rest)
				break
			}
		}
		if trimmed == s {
			return s
		}
		s = trimmed
	}
}

// prefixEnds reports whether a glyph is a whole prefix: followed by a space,
// or by nothing. A glyph glued to a word ("*args") is part of the title.
func prefixEnds(rest string) bool { return rest == "" || rest[0] == ' ' }

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:limit]))
}

// SanitisePushName returns the name deck may push into a running agent of
// kind: the harness-name sanitiser, then the harness's own limits (Copilot
// 1-100 runes with no `"`, Claude at most 200 runes). ok is false when
// nothing is left, because a blank name is never passed to a harness.
func SanitisePushName(kind, name string) (string, bool) {
	clean := collapseSpace(stripControl(stripEscapes(name)))
	switch kind {
	case "copilot":
		clean = truncateRunes(strings.ReplaceAll(clean, `"`, "'"), copilotNameMaxRunes)
	case "claude":
		clean = truncateRunes(clean, claudeNameMaxRunes)
	default:
		clean = truncateRunes(clean, MaxHarnessNameRunes)
	}
	return clean, clean != ""
}
