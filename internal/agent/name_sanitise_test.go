package agent

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// TestSanitiseHarnessName is R253's table: terminal escapes, control
// characters and a glyph prefix never survive, the result is bounded, and an
// all-junk title yields "" (= no adoption).
func TestSanitiseHarnessName(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain", "Fix the parser", "Fix the parser"},
		{"csi colour", "\x1b[31mFix\x1b[0m the parser", "Fix the parser"},
		{"osc title with BEL", "\x1b]0;evil\x07Fix it", "Fix it"},
		{"osc title with ST", "\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"lone escape at end", "Fix it\x1b", "Fix it"},
		{"two-byte escape", "a\x1bcb", "ab"},
		{"c0 controls", "Fix\x00\x01\x07\x08 it\x7f", "Fix it"},
		{"c1 controls", "Fix\u0085\u009b it\u009d", "Fix it"},
		{"newline and tab become a space", "Fix\nthe\tparser\r", "Fix the parser"},
		{"bidi override", "Fix \u202eti\u202c", "Fix ti"},
		{"claude glyph prefix", "✳ Fix the parser", "Fix the parser"},
		{"spinner prefix", "⠋ Fix the parser", "Fix the parser"},
		{"stacked prefix", "✳ ⠙ Fix the parser", "Fix the parser"},
		{"pi prefix", "π - Fix the parser", "Fix the parser"},
		{"glyph glued to a word is kept", "✳Fix", "✳Fix"},
		{"shell metacharacters are data", "$(rm -rf ~); `id`", "$(rm -rf ~); `id`"},
		{"all junk", "\x1b[2J\x07\x00 ✳ ", ""},
		{"prefix only", "π - ", ""},
		{"blank", "   ", ""},
	} {
		if got := SanitiseHarnessName(tc.in); got != tc.want {
			t.Errorf("%s: SanitiseHarnessName(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestSanitiseHarnessNameBoundsLengthInRunes(t *testing.T) {
	got := SanitiseHarnessName(strings.Repeat("é", MaxHarnessNameRunes+50))
	if n := utf8.RuneCountInString(got); n != MaxHarnessNameRunes {
		t.Fatalf("length = %d runes, want %d", n, MaxHarnessNameRunes)
	}
}

func TestSanitiseHarnessNameLeavesNoControlRune(t *testing.T) {
	var raw strings.Builder
	for r := rune(0); r < 0xa0; r++ {
		raw.WriteRune(r)
		raw.WriteRune('x')
	}
	for _, r := range SanitiseHarnessName(raw.String()) {
		if unicode.IsControl(r) {
			t.Fatalf("control rune %U survived", r)
		}
	}
}

// TestSanitisePushName is R253's per-harness push rule: Copilot takes 1-100
// runes and no `"`, Claude is capped at 200, and an empty result is refused.
func TestSanitisePushName(t *testing.T) {
	long := strings.Repeat("ü", 250)
	for _, tc := range []struct {
		name, kind, in string
		wantRunes      int
		want           string
		ok             bool
	}{
		{name: "copilot replaces the double quote", kind: "copilot", in: `say "hi"`, want: `say 'hi'`, ok: true},
		{name: "copilot clamps to 100 runes", kind: "copilot", in: long, wantRunes: 100, ok: true},
		{name: "copilot keeps a 1-rune name", kind: "copilot", in: "a", want: "a", ok: true},
		{name: "copilot refuses blank", kind: "copilot", in: " \x1b[0m ", ok: false},
		{name: "claude caps at 200 runes", kind: "claude", in: long, wantRunes: 200, ok: true},
		{name: "claude keeps a quote", kind: "claude", in: `say "hi"`, want: `say "hi"`, ok: true},
		{name: "claude strips escapes", kind: "claude", in: "\x1b[1mwork\x1b[0m\x07", want: "work", ok: true},
		{name: "claude refuses blank", kind: "claude", in: "\x00", ok: false},
	} {
		got, ok := SanitisePushName(tc.kind, tc.in)
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if tc.wantRunes > 0 && utf8.RuneCountInString(got) != tc.wantRunes {
			t.Errorf("%s: %d runes, want %d", tc.name, utf8.RuneCountInString(got), tc.wantRunes)
		}
	}
}

// TestSanitiseHarnessNameStripsInvisibleFormatRanges has one row per invisible
// format range beyond the bidi and zero-width marks (SPEC §8, R272).
func TestSanitiseHarnessNameStripsInvisibleFormatRanges(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"word joiner and invisible operators U+2060-U+2064", "Fix\u2060 the\u2061 par\u2062s\u2063e\u2064r", "Fix the parser"},
		{"soft hyphen U+00AD", "Fix the par\u00adser", "Fix the parser"},
		{"arabic letter mark U+061C", "Fix the\u061c parser", "Fix the parser"},
		{"mongolian vowel separator U+180E", "Fix the\u180e parser", "Fix the parser"},
		{"tag characters U+E0000-U+E007F", "Fix the \U000e0000par\U000e0041ser\U000e007f", "Fix the parser"},
	} {
		if got := SanitiseHarnessName(tc.in); got != tc.want {
			t.Errorf("%s: SanitiseHarnessName(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
