package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestCropRowNeverSplitsWideGlyph is task 019's core invariant (SPEC
// requirement 24): whatever contentWidth a panel resolves to (sidebar_width
// and terminal width both change it at runtime), cropRow's output is
// exactly that many display columns wide -- never one short (a wide glyph
// silently dropped in favour of a smaller total) and never one over (half
// of a wide glyph rendered, shearing the border that follows it). Task
// 008's East-Asian-Wide fixture line mixes width-2 CJK glyphs with width-1
// box-drawing glyphs specifically so a boundary that lands mid-glyph is
// exercised at multiple offsets, not just one lucky width.
func TestCropRowNeverSplitsWideGlyph(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	raw := previewFixture(t, "wide.txt")
	rows := splitPreviewLines(raw)
	if len(rows) < 4 {
		t.Fatalf("wide.txt has %d rows, want at least 4", len(rows))
	}
	// rows[2] alternates 界(width 2) and ┌(width 1): 界┌界┌界┌界┌界┌ -- a
	// glyph boundary falls on nearly every odd column, so this line
	// exercises the split case densely.
	mixed := rows[2]
	if stringWidth(mixed) < 15 {
		t.Fatalf("fixture row %q has display width %d, want >= 15 (fixture drifted)", mixed, stringWidth(mixed))
	}
	marker := m.cropMarker()
	markerW := stringWidth(marker)
	for contentWidth := 1; contentWidth <= 22; contentWidth++ {
		got := m.cropRow(mixed, contentWidth)
		if gotW := stringWidth(got); gotW != contentWidth {
			t.Fatalf("cropRow(%q, %d) = %q, display width %d, want exactly %d (a sheared border)", mixed, contentWidth, got, gotW, contentWidth)
		}
		// Every rune in the result must be a whole rune from the source
		// (or a space we padded, or the marker) -- rebuild the result from
		// whole runes and confirm it round-trips through width accounting
		// with no fractional glyph: summing cellWidth over the runes we
		// actually emitted must equal contentWidth exactly, which is only
		// possible if no rune was cut in half (a half-emitted multi-byte
		// rune cannot exist in a valid UTF-8 string in the first place, so
		// this also guards against padding math emitting garbage bytes).
		sum := 0
		for _, r := range got {
			sum += cellWidth(r)
		}
		if sum != contentWidth {
			t.Fatalf("cropRow(%q, %d) rune-width sum = %d, want %d", mixed, contentWidth, sum, contentWidth)
		}
		// If the row was actually cropped (its full width exceeds the
		// budget) and there is room for the marker, the marker must be
		// the last thing in the string, occupying a column of its own --
		// never merged into or splitting a preceding wide glyph.
		if stringWidth(mixed) > contentWidth && contentWidth >= markerW {
			if !strings.HasSuffix(got, marker) {
				t.Fatalf("cropRow(%q, %d) = %q, want to end with crop marker %q", mixed, contentWidth, got, marker)
			}
		}
	}
}

// TestCropPreviewBottomLeftWideFixtureKeepsBorderColumn exercises the same
// invariant through the full cropPreviewBottomLeft entry point (rather than
// the cropRow helper directly) against every one of wide.txt's three
// distinct lines (all-wide, wide/narrow-alternating, all-wide) at several
// panel widths smaller than the fixture's own content -- the exact
// situation SPEC requirement 24 calls out: "the panel border stays in the
// same column for every crop offset".
func TestCropPreviewBottomLeftWideFixtureKeepsBorderColumn(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	raw := previewFixture(t, "wide.txt")
	const realWidth, realHeight = 20, 4
	for _, contentWidth := range []int{5, 9, 10, 11, 15, 19, 20} {
		lines := m.cropPreviewBottomLeft(raw, contentWidth, realHeight, realWidth, realHeight)
		for i, line := range lines {
			if got := stringWidth(line); got != contentWidth {
				t.Fatalf("contentWidth=%d lines[%d] = %q, display width %d, want %d", contentWidth, i, line, got, contentWidth)
			}
			if got := stringWidth(line); got != len([]rune(line)) {
				// A line whose display width differs from its rune count
				// still has every border after it landing at the same
				// column only because we already checked display width
				// above; this branch documents that fact rather than
				// re-asserting it, so no failure path exists here beyond
				// the check already performed.
				_ = got
			}
		}
	}
}

// TestPadTruncElidesWideSessionNameWithoutSplitting covers the sidebar-name
// half of requirement 24: padTrunc backs sidebarContentLine, so a session
// name containing East-Asian-Wide characters must truncate the same
// whole-glyph-or-nothing way the preview crop does, and the result must
// still be exactly the requested width so the sidebar's own border never
// shears either.
func TestPadTruncElidesWideSessionNameWithoutSplitting(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	name := "界界界界界界界界界界" // 10 runes, width 20 -- task 008's wide fixture text
	for width := 1; width <= 22; width++ {
		got := m.padTrunc(name, width)
		if gotW := stringWidth(got); gotW != width {
			t.Fatalf("padTrunc(%q, %d) = %q, display width %d, want %d", name, width, got, gotW, width)
		}
		sum := 0
		for _, r := range got {
			sum += cellWidth(r)
		}
		if sum != width {
			t.Fatalf("padTrunc(%q, %d) rune-width sum = %d, want %d (half a glyph emitted)", name, width, sum, width)
		}
	}
}
