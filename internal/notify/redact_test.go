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

func TestRedactMasksSecretPairsAndSessionValues(t *testing.T) {
	env := map[string]string{"API_TOKEN": "tok-9f2", "REGION": "eu-north-7", "N": "12", "EMPTY": "", "LONG": "eu-north-7-long"}
	for text, want := range map[string]string{
		`TOKEN=abc and "x"`:                     "TOKEN=" + MaskedPlaceholder + ` and "x"`,
		`password: 'hun ter2' ok`:               "password: " + MaskedPlaceholder + " ok",
		`secret = "a b"`:                        "secret = " + MaskedPlaceholder,
		"build=ok 12 steps":                     "build=ok 12 steps",
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
