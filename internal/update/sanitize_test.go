package update

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// controlCorpus is R330's seven inputs, the C1 CSI byte both raw (0x9B, the
// form an 8-bit terminal acts on) and UTF-8 encoded (C2 9B): what each is
// before and after the control characters are removed. "ě" is C4 9B, so the
// multi-byte row also holds a 0x9B byte that is part of a valid character.
var controlCorpus = []struct{ name, raw, want string }{
	{"ESC", "ok\x1b[31mred", "ok[31mred"},
	{"BEL", "a\x07b", "ab"},
	{"C1 CSI byte", "a\x9b31mb", "a31mb"},
	{"C1 CSI UTF-8 encoded", "a\u009b31mb", "a31mb"},
	{"OSC title sequence", "x\x1b]0;pwned\x07y", "x]0;pwnedy"},
	{"newline", "line1\nline2", "line1line2"},
	{"normal string", "503 Service Unavailable", "503 Service Unavailable"},
	{"multi-byte string", "ошибка 配信 ✓ é ě", "ошибка 配信 ✓ é ě"},
}

// hasControl reports a C0, DEL or C1 rune, or a raw byte 0x80-0x9F outside a
// valid UTF-8 sequence, anywhere in s.
func hasControl(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			if s[i] >= 0x80 && s[i] <= 0x9f {
				return true
			}
		} else if r <= 0x1f || r >= 0x7f && r <= 0x9f {
			return true
		}
		i += size
	}
	return false
}

func TestStripControl(t *testing.T) {
	for _, c := range controlCorpus {
		t.Run(c.name, func(t *testing.T) {
			if got := StripControl(c.raw); got != c.want {
				t.Fatalf("StripControl(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
	// The whole range is removed: C0, DEL and C1, and nothing next to it.
	for r := rune(0); r <= 0xa0; r++ {
		got := StripControl("a" + string(r) + "b")
		want := "ab"
		if r > 0x1f && r < 0x7f || r == 0xa0 {
			want = "a" + string(r) + "b"
		}
		if got != want {
			t.Errorf("U+%04X: got %q, want %q", r, got, want)
		}
	}
	// A lone byte 0x80-0x9F (not part of a valid UTF-8 sequence) is a C1
	// control to an 8-bit terminal and is removed; any other lone byte stays.
	for c := 0x80; c <= 0xff; c++ {
		in := "a" + string([]byte{byte(c)}) + "b"
		want := "ab"
		if c > 0x9f {
			want = in
		}
		if got := StripControl(in); got != want {
			t.Errorf("raw byte 0x%02X: got %q, want %q", c, got, want)
		}
	}
	// After an invalid lead byte the C1 byte is still removed; inside a valid
	// sequence the same byte is kept.
	for in, want := range map[string]string{
		"x\xe0\x9by":       "x\xe0y",
		"x\x9b\x9b[2Jy":    "x[2Jy",
		"x\u011b\x9by":     "x\u011by",
		"\x9b":             "",
		"\xc2\x9b\x1b\x9b": "",
	} {
		if got := StripControl(in); got != want {
			t.Errorf("StripControl(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestServerTextErrorsCarryNoControlCharacters: a checksums.txt field, a
// redirect path and a gh-style cause that hold control characters reach the
// error message without them (the message every sink prints).
func TestServerTextErrorsCarryNoControlCharacters(t *testing.T) {
	const cfg = "https://mirror.example/base"
	for _, c := range controlCorpus {
		t.Run(c.name, func(t *testing.T) {
			for name, err := range map[string]error{
				"checksum field": fmt.Errorf("checksum mismatch: checksums.txt says %w, the download is x", serverString(c.raw)),
				"redirect path":  fmt.Errorf("redirect %w does not name a stable tag", serverString(c.raw)),
				"gh stderr":      fmt.Errorf("gh: %w", fromServer(errors.New(c.raw))),
			} {
				got := redactErr(err, cfg).Error()
				if hasControl(got) {
					t.Errorf("%s: %q still holds a control character", name, got)
				}
				if !strings.Contains(got, c.want) {
					t.Errorf("%s: %q lost the text %q", name, got, c.want)
				}
			}
		})
	}
}

// TestRedactionStillAppliesToSanitisedText: a credential the server echoes
// with a control character inside it is whole again once the character is
// gone, and is still masked; the control character does not hide it.
func TestRedactionStillAppliesToSanitisedText(t *testing.T) {
	const secret = "s3cretvalue"
	cfg := "https://mirror.example/base?token=" + secret
	for name, echoed := range map[string]string{
		"ESC inside":     "s3cr\x1betvalue",
		"newline inside": "s3cret\nvalue",
		"C1 inside":      "s3\u009bcretvalue",
		"raw C1 inside":  "s3\x9bcretvalue",
		"whole":          secret,
	} {
		err := redactErr(fmt.Errorf("checksums.txt says %w", serverString("x"+echoed+"y")), cfg)
		got := err.Error()
		if strings.Contains(got, secret) || hasControl(got) || !strings.Contains(got, redacted) {
			t.Errorf("%s: %q leaks the secret, holds a control character or was not masked", name, got)
		}
	}
	// A one-byte secret an echo splits the same way.
	err := redactErr(fmt.Errorf("says %w", serverString("a\x1bZb")), "https://m.example/x?t=Z")
	if strings.Contains(err.Error(), "Z") || hasControl(err.Error()) {
		t.Errorf("short secret: %q", err.Error())
	}
}
