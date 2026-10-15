package update

import "strings"

// StripControl removes the C0 (U+0000-U+001F), DEL (U+007F) and C1
// (U+0080-U+009F) control characters from s. Text a release server or gh sent
// (a redirect path, a reason phrase, a checksums.txt field, stderr) reaches a
// terminal through the footer, the ctrl+u dialog and `deck update`'s stderr,
// where an ESC, a BEL, a C1 CSI or an OSC title sequence would be acted on
// instead of shown. Printable and multi-byte UTF-8 text is kept as it is, so
// is any other byte sequence; the characters the sequence used are removed, a
// newline included. Strip before redacting: removing a control byte can join
// the halves of a credential an echo split with one, and the redaction must
// see the joined text.
func StripControl(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	return r <= 0x1f || (r >= 0x7f && r <= 0x9f)
}
