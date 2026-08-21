package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestStringWidthIgnoresANSIEscapes covers the review finding "escape bytes
// counted as columns" (requirements 22/24): stringWidth used to be plain
// runewidth.StringWidth on the raw captured-pane bytes, so an SGR sequence
// like "\x1b[31m" spent 4 of the panel's columns even though it draws
// nothing. Coloured text's *visible* width must be exactly the width of the
// text a viewer actually sees.
func TestStringWidthIgnoresANSIEscapes(t *testing.T) {
	s := "\x1b[31mRED\x1b[0m"
	if got := stringWidth(s); got != 3 {
		t.Fatalf("stringWidth(%q) = %d, want 3 (escape bytes must not spend columns)", s, got)
	}
}

// TestTruncateToWidthKeepsEscapeBytesItPassesOver asserts truncateToWidth
// preserves every escape sequence it walks over byte-for-byte (so the
// colour it carries still reaches the terminal for whatever visible text
// survives the truncation) while only ever counting printable runes
// against the budget.
func TestTruncateToWidthKeepsEscapeBytesItPassesOver(t *testing.T) {
	s := "\x1b[31mRED\x1b[0m"
	got := truncateToWidth(s, 3)
	if !strings.Contains(got, "\x1b[31m") {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want the leading SGR escape preserved", s, got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want the trailing SGR escape preserved (all 3 visible cells fit budget 3)", s, got)
	}
	if !strings.HasSuffix(got, "RED\x1b[0m") {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want visible text RED intact with its reset escape", s, got)
	}
	if gotW := stringWidth(got); gotW != 3 {
		t.Fatalf("truncateToWidth(%q, 3) = %q, visible width %d, want 3", s, got, gotW)
	}

	// A tighter budget still keeps every escape byte it passes over, but
	// stops emitting visible runes once the budget is spent.
	got2 := truncateToWidth(s, 2)
	if !strings.Contains(got2, "\x1b[31m") {
		t.Fatalf("truncateToWidth(%q, 2) = %q, want the leading SGR escape preserved", s, got2)
	}
	if !strings.HasSuffix(got2, "RE") {
		t.Fatalf("truncateToWidth(%q, 2) = %q, want visible text truncated to RE", s, got2)
	}
	if gotW := stringWidth(got2); gotW != 2 {
		t.Fatalf("truncateToWidth(%q, 2) = %q, visible width %d, want 2", s, got2, gotW)
	}
}

// TestTruncateToWidthStillNeverSplitsWideGlyph guards against a regression
// where escape-awareness broke the existing whole-glyph-or-nothing rule
// (SPEC requirement 24) for wide East-Asian glyphs mixed with colour.
func TestTruncateToWidthStillNeverSplitsWideGlyph(t *testing.T) {
	s := "\x1b[32m界界界\x1b[0m"
	for budget := 1; budget <= 6; budget++ {
		got := truncateToWidth(s, budget)
		sum := 0
		for i := 0; i < len(got); {
			if got[i] == 0x1b {
				i += ansiEscapeLen(got, i)
				continue
			}
			r, size := utf8.DecodeRuneInString(got[i:])
			sum += cellWidth(r)
			i += size
		}
		if gotW := stringWidth(got); gotW != sum {
			t.Fatalf("truncateToWidth(%q, %d) = %q, stringWidth %d != rune-width sum %d", s, budget, got, gotW, sum)
		}
		if gotW := stringWidth(got); gotW > budget {
			t.Fatalf("truncateToWidth(%q, %d) = %q, visible width %d exceeds budget", s, budget, got, gotW)
		}
		if !strings.Contains(got, "\x1b[32m") {
			t.Fatalf("truncateToWidth(%q, %d) = %q, want the leading escape preserved even for budget %d", s, budget, got, budget)
		}
	}
}
