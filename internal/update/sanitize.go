package update

import (
	"strings"
	"unicode/utf8"
)

// StripControl removes the C0 (U+0000-U+001F), DEL (U+007F) and C1
// (U+0080-U+009F) control characters from s. Text a release server or gh sent
// (a redirect path, a reason phrase, a checksums.txt field, stderr) reaches a
// terminal through the footer, the ctrl+u dialog and `deck update`'s stderr,
// where an ESC, a BEL, a C1 CSI or an OSC title sequence would be acted on
// instead of shown. A C1 control arrives either UTF-8 encoded (U+009B is the
// bytes C2 9B) or as one raw byte (0x9B, which an 8-bit terminal reads as
// CSI); both forms are removed: every byte 0x80-0x9F that is not part of a
// valid UTF-8 sequence goes. Printable and multi-byte UTF-8 text is kept as it
// is, so is any other byte; the characters a sequence used are removed, a
// newline included. Strip before redacting: removing a control byte can join
// the halves of a credential an echo split with one, and the redaction must
// see the joined text.
func StripControl(s string) string {
	if !containsControl(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !controlAt(r, size, s[i]) {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// containsControl reports whether s holds a character StripControl removes.
func containsControl(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if controlAt(r, size, s[i]) {
			return true
		}
		i += size
	}
	return false
}

// controlAt reports whether the character decoded as r (size bytes, starting
// with first) is a control: a valid C0/DEL/C1 rune, or a lone byte 0x80-0x9F
// that is not part of a valid UTF-8 sequence.
func controlAt(r rune, size int, first byte) bool {
	if r == utf8.RuneError && size == 1 {
		return first >= 0x80 && first <= 0x9f
	}
	return isControl(r)
}

func isControl(r rune) bool {
	return r <= 0x1f || (r >= 0x7f && r <= 0x9f)
}
