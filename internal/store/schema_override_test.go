package store

import "testing"

func TestParseSchemaOverride(t *testing.T) {
	const fallback = 8
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"valid number", "7", 7},
		{"non-numeric", "seven", fallback},
		{"empty", "", fallback},
		{"zero", "0", fallback},
		{"negative", "-3", fallback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSchemaOverride(c.in, fallback); got != c.want {
				t.Fatalf("parseSchemaOverride(%q, %d) = %d, want %d", c.in, fallback, got, c.want)
			}
		})
	}
}
