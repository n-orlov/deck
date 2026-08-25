package tui

import (
	"strings"
	"testing"
)

// TestTruncateToWidthClosesOpenBackgroundSpanMidRun is task 320's core
// case (SPEC §11.3: "every truncated coloured run re-emits its own
// reset"): settingsRenderRow (settings.go) opens a row's selection/surface
// BACKGROUND span once and closes it with a single trailing reset only
// after every segment's text — so a caller that truncates the composed
// line before reaching that reset (padTrunc -> truncateToWidth, when the
// row is too long for the panel) used to hand back a string whose
// background was still open, left for whatever the caller concatenates
// next (the ellipsis, padding, the seam) to inherit. The fix must re-emit
// the reset itself, at zero visible cost, whenever the cut point falls
// inside a still-open background span.
func TestTruncateToWidthClosesOpenBackgroundSpanMidRun(t *testing.T) {
	s := "\x1b[48;2;10;20;30mHELLO\x1b[0m"
	got := truncateToWidth(s, 3)

	if !strings.Contains(got, "\x1b[48;2;10;20;30m") {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want the opening background escape preserved", s, got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want a trailing reset re-emitted (the source's own reset sat past the cut point and was dropped)", s, got)
	}
	if visible := stripANSI(got); visible != "HEL" {
		t.Fatalf("truncateToWidth(%q, 3) visible text = %q, want %q", s, visible, "HEL")
	}
	if w := stringWidth(got); w > 3 {
		t.Fatalf("truncateToWidth(%q, 3) = %q, visible width %d exceeds budget 3 (the synthesised reset must cost zero columns)", s, got, w)
	}

	// The source never closed the span at all (no "\x1b[0m" anywhere) --
	// the same defect must be fixed even when nothing in s was ever
	// dropped by truncation, i.e. an open span left open at the natural
	// end of the string is exactly as much a leak as one cut off
	// mid-run.
	unterminated := "\x1b[48;2;10;20;30mHI"
	got2 := truncateToWidth(unterminated, 10)
	if !strings.HasSuffix(got2, "\x1b[0m") {
		t.Fatalf("truncateToWidth(%q, 10) = %q, want a trailing reset re-emitted (source never closed its own background span)", unterminated, got2)
	}
	if visible := stripANSI(got2); visible != "HI" {
		t.Fatalf("truncateToWidth(%q, 10) visible text = %q, want %q", unterminated, visible, "HI")
	}
}

// TestTruncateToWidthLeavesPlainTextAlone is the no-escape sanity case:
// truncation of a string that never opens any SGR span must not
// synthesise a reset it never needed, so a caller with NO_COLOR (or that
// simply never colours a run) keeps getting exactly plain truncated text.
func TestTruncateToWidthLeavesPlainTextAlone(t *testing.T) {
	got := truncateToWidth("HELLO", 3)
	if got != "HEL" {
		t.Fatalf("truncateToWidth(%q, 3) = %q, want %q with no reset synthesised", "HELLO", got, "HEL")
	}
}

// TestTruncateToWidthDoesNotDoubleCloseAnAlreadyClosedBackground is the
// escape-only-tail case: when every visible rune fits inside budget and
// the only thing left in s past the last one is the run's own closing
// reset (a pure-escape tail, costing zero columns), the existing loop
// already copies it through without ever breaking early. The fix must
// recognise that reset as closing the span it tracks, so it does not
// ALSO append a second, redundant one.
func TestTruncateToWidthDoesNotDoubleCloseAnAlreadyClosedBackground(t *testing.T) {
	s := "\x1b[48;5;236mHI\x1b[0m"
	got := truncateToWidth(s, 10) // budget comfortably covers "HI" (width 2)
	want := s
	if got != want {
		t.Fatalf("truncateToWidth(%q, 10) = %q, want %q unchanged (the source's own reset already closes the span; no second reset should be appended)", s, got, want)
	}
	if n := strings.Count(got, "\x1b[0m"); n != 1 {
		t.Fatalf("truncateToWidth(%q, 10) = %q, want exactly one reset, got %d", s, got, n)
	}
}

// TestTruncateToWidthWideGlyphsStillCloseTheirBackground guards the
// existing whole-glyph-or-nothing rule (SPEC requirement 24) against a
// regression from the new background-closing logic: a wide-glyph run with
// an open background must still never split a glyph in half, and must
// still close the span at every budget level that cuts the run short.
func TestTruncateToWidthWideGlyphsStillCloseTheirBackground(t *testing.T) {
	s := "\x1b[48;2;5;5;5m\u754c\u754c\u754c" // background open, three wide glyphs, never closed in source
	for budget := 1; budget <= 6; budget++ {
		got := truncateToWidth(s, budget)
		if w := stringWidth(got); w > budget {
			t.Fatalf("truncateToWidth(%q, %d) = %q, visible width %d exceeds budget", s, budget, got, w)
		}
		if !strings.HasSuffix(got, "\x1b[0m") {
			t.Fatalf("truncateToWidth(%q, %d) = %q, want the still-open background span closed with a trailing reset", s, budget, got)
		}
	}
}
