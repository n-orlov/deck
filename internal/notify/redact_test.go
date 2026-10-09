package notify

import (
	"strings"
	"testing"
)

func TestIsSecretShapedKeyMatchesSPEC64Pattern(t *testing.T) {
	for key, want := range map[string]bool{
		"API_TOKEN": true, "client_secret": true, "AWS_KEY": true, "DB_PASSWORD": true,
		"my.credential": true, "REGION": false, "PATH": false, "": false,
	} {
		if got := IsSecretShapedKey(key); got != want {
			t.Errorf("IsSecretShapedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

// The allocation-free ASCII path must agree with the upper-casing reference
// (SPEC §6.4: case-insensitive substring) on mixed case, near misses and keys
// with non-ASCII letters, which take the Unicode path.
func TestIsSecretShapedKeyAgreesWithTheUpperCasingReference(t *testing.T) {
	for _, key := range []string{
		"ToKeN", "pAsSwOrD_x", "x-secret-y", "SECRE", "TOKE", "KE", "k_e_y", "credentia",
		"CREDENTIALS", "naïve_key", "ſecret", "tokën", "ΚEY", "a.b-c", "key",
	} {
		upper := strings.ToUpper(key)
		want := false
		for _, substr := range secretShapedKeySubstrings {
			want = want || strings.Contains(upper, substr)
		}
		if got := IsSecretShapedKey(key); got != want {
			t.Errorf("IsSecretShapedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestRedactMasksSecretPairsAndSessionValues(t *testing.T) {
	env := map[string]string{"API_TOKEN": "tok-9f2", "REGION": "eu-north-7", "N": "12", "ONE": "Z", "EMPTY": "", "LONG": "eu-north-7-long"}
	for text, want := range map[string]string{
		`TOKEN=abc and "x"`:                     "TOKEN=" + MaskedPlaceholder + ` and "x"`,
		`password: 'hun ter2' ok`:               "password: " + MaskedPlaceholder + " ok",
		`secret = "a b"`:                        "secret = " + MaskedPlaceholder,
		"build=ok 12 steps":                     "build=ok " + MaskedPlaceholder + " steps",
		"short Z value":                         "short " + MaskedPlaceholder + " value",
		"nothing shaped like a value":           "nothing shaped like a value",
		"region eu-north-7 and eu-north-7-long": "region " + MaskedPlaceholder + " and " + MaskedPlaceholder,
		"used tok-9f2 here":                     "used " + MaskedPlaceholder + " here",
	} {
		if got := Redact(text, env); got != want {
			t.Errorf("Redact(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short text changed: %q", got)
	}
	if got := truncate("abc", 0); got != "abc" {
		t.Errorf("zero limit must mean no cap, got %q", got)
	}
	got := truncate(strings.Repeat("é", 5), 3)
	if got != "é…" {
		t.Errorf("truncate = %q, want a whole rune then the marker", got)
	}
}

func TestTailBufferKeepsTheLastBytesOnARuneStart(t *testing.T) {
	b := newTailBuffer(5)
	_, _ = b.Write([]byte("héllo wörld"))
	got, dropped := b.tail()
	if !dropped || got != "örld" {
		t.Errorf("tail = %q dropped %v, want the last whole runes", got, dropped)
	}
	if got, trunc := capTail("abcdef", 3, false); got != "def" || !trunc {
		t.Errorf("capTail = %q %v", got, trunc)
	}
	if got, trunc := capTail("éé", 3, false); got != "é" || !trunc {
		t.Errorf("capTail rune = %q %v", got, trunc)
	}
}
