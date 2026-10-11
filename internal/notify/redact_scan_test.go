package notify

import (
	"regexp"
	"strings"
	"testing"
)

// oracleAssignment is the regular expression the linear scanner replaced; it
// is the specification of which pairs the scanner must find, with one
// extension: a backslash escapes the byte after it inside a quoted value.
var oracleAssignment = regexp.MustCompile(`([A-Za-z0-9_.\-]+)(\s*[=:]\s*)("(?:[^"\\]|\\(?s:.))*"|'(?:[^'\\]|\\(?s:.))*'|\S+)`)

func oracleMask(text string) string {
	return oracleAssignment.ReplaceAllStringFunc(text, func(match string) string {
		parts := oracleAssignment.FindStringSubmatch(match)
		if !IsSecretShapedKey(parts[1]) {
			return match
		}
		return parts[1] + parts[2] + MaskedPlaceholder
	})
}

func TestMaskKeyValuePairsAgreesWithTheRegexpItReplaced(t *testing.T) {
	for _, text := range []string{
		"", "plain words only", "API_TOKEN=abc", "API_TOKEN = abc def", "api-key: \"two words\" next",
		"password='a b' tail", "password='unterminated tail", `token="unterminated tail`, "TOKEN=", "TOKEN=   ",
		"=TOKEN", ": x", "a=b=c", "SECRET_X=a:b SECRET_Y:c", "x_token=1\nPASSWORD:\n2", "TOKEN=\"a\"rest AUTH=z",
		`token="a\"b" rest`, `token='a\\' rest`, `token="a\`, "é_TOKEN=v ünï=TOKEN", "my.secret-key:v", "TOKEN==v", "TOKEN=:v", "a TOKEN\t=\tv b", "KEY=\x00\xff bin",
	} {
		if got, want := maskKeyValuePairs(text), oracleMask(text); got != want {
			t.Errorf("maskKeyValuePairs(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestMaskKeyValuePairsAgreesWithTheRegexpOnEveryShortSequence(t *testing.T) {
	pieces := []string{"TOKEN", "k", "=", ":", " ", "\n", `"`, "'", "-", "é", "\xff", `\`}
	var walk func(prefix string, depth int)
	walk = func(prefix string, depth int) {
		if got, want := maskKeyValuePairs(prefix), oracleMask(prefix); got != want {
			t.Fatalf("maskKeyValuePairs(%q) = %q, want %q", prefix, got, want)
		}
		if depth == 0 {
			return
		}
		for _, piece := range pieces {
			walk(prefix+piece, depth-1)
		}
	}
	walk("", 5)
}

// A reason of megabytes made of nothing but separators and pairs is the
// worst case for the scan: it must stay linear and still mask every secret.
func TestMaskKeyValuePairsMasksEveryPairInAVeryLongText(t *testing.T) {
	text := strings.Repeat("name=ok API_TOKEN=hunter2 ::: = ", 1<<15)
	got := maskKeyValuePairs(text)
	if strings.Contains(got, "hunter2") || strings.Count(got, "API_TOKEN="+MaskedPlaceholder) != 1<<15 {
		t.Fatalf("a secret survived or a pair was lost in a %d-byte text", len(text))
	}
	if want := oracleMask(text); got != want {
		t.Fatal("long text differs from the regexp")
	}
}

// Longer texts than the exhaustive walk reaches, drawn from the same pieces
// plus every kind of whitespace and a spread of key shapes, against the
// regexp oracle: the scan finds pairs through strings.Trim/Index calls, and
// this is what pins their boundaries (a key behind several spaces, a value
// behind a tab, a quote that never closes, a separator run).
func TestMaskKeyValuePairsAgreesWithTheRegexpOnRandomTexts(t *testing.T) {
	pieces := []string{
		"TOKEN", "api_key", "k", "v", "=", ":", "=", ":", " ", "  ", "\t", "\n", "\r", "\f", `"`, "'", "-", ".", "é", "\xff",
		"PASSWORD", "x y", "secret", "a=b", "::", "= ", `\`, `\"`, `\'`,
	}
	// A fixed linear congruential stream: the same texts every run.
	state := 74
	next := func(n int) int {
		state = (state*1103515245 + 12345) & 0x7fffffff
		return state >> 8 % n
	}
	for i := 0; i < 30000; i++ {
		var b strings.Builder
		for n := next(14); n >= 0; n-- {
			b.WriteString(pieces[next(len(pieces))])
		}
		text := b.String()
		if got, want := maskKeyValuePairs(text), oracleMask(text); got != want {
			t.Fatalf("maskKeyValuePairs(%q) = %q, want %q", text, got, want)
		}
	}
}
