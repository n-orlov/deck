package interactive

import (
	"bytes"
	"testing"
)

// stringKinds lists every escape-string kind with the byte sequences that
// reach each of its header and payload states (R209). Each opener ends in a
// state from which ESC \ must end the string.
var stringKinds = map[string]string{
	"OSC":                 "\x1b]",
	"OSC payload":         "\x1b]0;title",
	"SOS":                 "\x1bX",
	"SOS payload":         "\x1bXtext",
	"PM":                  "\x1b^",
	"PM payload":          "\x1b^text",
	"APC":                 "\x1b_",
	"APC payload":         "\x1b_text",
	"empty DCS":           "\x1bP",
	"DCS params":          "\x1bP1;2",
	"DCS intermediate":    "\x1bP$",
	"DCS params then int": "\x1bP1$",
	"DCS payload":         "\x1bPqabc",
	"DCS ESC payload":     "\x1bP\x1b\x1b",
	"DCS payload BEL":     "\x1bPq\x07",
}

// TestEscBackslashEndsEveryStringKindInEveryStateAtEverySplit asserts the
// bytes after ESC \ -- non-ASCII text and a raw C1 byte -- come through
// unchanged however the stream is split across writes.
func TestEscBackslashEndsEveryStringKindInEveryStateAtEverySplit(t *testing.T) {
	const after = "é✳日本\x9c\x9d ok"
	for name, opener := range stringKinds {
		data := []byte(opener + "\x1b\\" + after)
		for _, chunks := range splitEverywhere(data) {
			var f stringFilter
			var got []byte
			for _, c := range chunks {
				got = append(got, f.filter(append([]byte(nil), c...))...)
			}
			if !bytes.HasSuffix(got, []byte("\x1b\\"+after)) {
				t.Errorf("%s: split %q: output %q lost or altered bytes after ESC \\", name, chunks, got)
			}
		}
	}
}

// TestEmptyDCSEndsOnEscBackslashButKeepsEscAsPayload covers the empty DCS
// (ESC P ESC \), a doubled ESC before the backslash, and an ESC followed by
// payload, which the vt table still treats as inside the string.
func TestEmptyDCSEndsOnEscBackslashButKeepsEscAsPayload(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty DCS then text", "\x1bP\x1b\\é✳\x9c", "\x1bP\x1b\\é✳\x9c"},
		{"empty DCS then title", "\x1bP\x1b\\\x1b]0;✳ t\x07✳", "\x1bP\x1b\\\x1b]0;\xe2\xb3 t\x07✳"},
		{"doubled ESC before backslash", "\x1bP\x1b\x1b\\é✳", "\x1bP\x1b\x1b\\é✳"},
		{"ESC then payload stays in the string", "\x1bP\x1bx✳", "\x1bP\x1bx\xe2\xb3"},
		{"SOS empty", "\x1bX\x1b\\é✳", "\x1bX\x1b\\é✳"},
		{"APC with C1 payload then text", "\x1b_a✳\x1b\\✳", "\x1b_a\x1b\\✳"},
		{"PM split header", "\x1b^\x1b\\✳", "\x1b^\x1b\\✳"},
		{"DCS with CAN before text", "\x1bP\x18✳", "\x1bP\x18✳"},
	}
	for _, c := range cases {
		var f stringFilter
		if got := string(f.filter([]byte(c.in))); got != c.want {
			t.Errorf("%s: filter(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
