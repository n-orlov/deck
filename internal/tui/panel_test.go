package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSideBySideFrameHasOneSeamAndOneColumnPadding proves SPEC requirements
// 17 and 18: the sidebar and preview share exactly one vertical bar between
// them (never "││"), and each panel indents its content by exactly one
// column of padding from its own border.
func TestSideBySideFrameHasOneSeamAndOneColumnPadding(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 100, 30
	view := model.View()
	lines := strings.Split(view, "\n")

	if strings.Contains(view, "││") {
		t.Fatalf("frame contains a doubled seam \"││\":\n%s", view)
	}
	if got := strings.Count(view, "│"); got == 0 {
		t.Fatalf("frame has no vertical border at all:\n%s", view)
	}

	// The top border line is "╭ deck — sessions ───...───┬───...───╮":
	// exactly one seam glyph, drawn at the T-junction where the sidebar's
	// top border meets the preview's, never two adjacent verticals.
	top := lines[0]
	if !strings.Contains(top, "┬") {
		t.Fatalf("top border missing the seam T-junction ┬:\n%q", top)
	}
	if strings.Count(top, "┬") != 1 {
		t.Fatalf("top border has %d seam T-junctions, want exactly 1:\n%q", strings.Count(top, "┬"), top)
	}

	// A content row reads "│ <sidebar text...>│ <preview text...> │":
	// left border, one padding space, then the seam (preview's own left
	// border) directly followed by one more padding space before the
	// preview's own text.
	contentRow := lines[1]
	if !strings.HasPrefix(contentRow, "│ ") {
		t.Fatalf("sidebar content row missing its border+one-column padding prefix:\n%q", contentRow)
	}
	afterFirst := contentRow[len("│"):]
	relIdx := strings.Index(afterFirst, "│")
	if relIdx < 0 {
		t.Fatalf("content row has no second (seam) border:\n%q", contentRow)
	}
	rest := afterFirst[relIdx+len("│"):]
	if !strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "  ") {
		t.Fatalf("preview content is not padded by exactly one column after the seam:\n%q", contentRow)
	}
	if strings.Count(contentRow, "││") > 0 {
		t.Fatalf("content row has an adjacent double bar:\n%q", contentRow)
	}
}

// TestSideBySideFrameASCIIFallbackHasNoUnicodeBorders proves the DECK_ASCII
// fallback (SPEC requirement 16) replaces every rounded-border glyph,
// including the seam, with plain ASCII, and keeps the same one-seam shape.
func TestSideBySideFrameASCIIFallbackHasNoUnicodeBorders(t *testing.T) {
	model := New(nil, config.Settings{ASCII: true}, "")
	model.width, model.height = 100, 30
	view := model.View()

	for _, glyph := range []string{"╭", "╮", "╰", "╯", "│", "─", "┬", "┴"} {
		if strings.Contains(view, glyph) {
			t.Fatalf("ASCII fallback frame still contains unicode border glyph %q:\n%s", glyph, view)
		}
	}
	// ASCII corners are also "+", so the top border reads
	// "+...+...+" (left corner, seam, right corner) — the seam is the
	// one "+" strictly between the two corners, not doubled with a
	// second adjacent junction glyph.
	lines := strings.Split(view, "\n")
	top := lines[0]
	if len(top) < 2 || top[0] != '+' || top[len(top)-1] != '+' {
		t.Fatalf("ASCII top border missing its corner glyphs:\n%q", top)
	}
	interior := top[1 : len(top)-1]
	if strings.Count(interior, "+") != 1 {
		t.Fatalf("ASCII top border has %d interior seam junctions, want exactly 1:\n%q", strings.Count(interior, "+"), top)
	}
	if strings.Contains(view, "||") {
		t.Fatalf("ASCII frame contains a doubled seam \"||\":\n%s", view)
	}
}

// TestEmptyStateAndPressNCopyLiveInsideSidebarAt80x24 proves SPEC's stated
// legibility floor: deck's supported minimum 80x24 still shows the empty
// state and "Press n" copy inside the sidebar panel, plus a preview
// placeholder, rather than losing either to the panel chrome.
func TestEmptyStateAndPressNCopyLiveInsideSidebarAt80x24(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	view := model.View()
	lines := strings.Split(view, "\n")

	if len(lines) == 0 {
		t.Fatalf("empty frame at 80x24")
	}
	foundEmpty, foundPressN, foundPlaceholder := false, false, false
	for _, line := range lines {
		if strings.Contains(line, "No sessions yet") {
			foundEmpty = true
		}
		if strings.Contains(line, "Press n") {
			foundPressN = true
		}
		if strings.Contains(line, "Select or create a session") {
			foundPlaceholder = true
		}
	}
	if !foundEmpty || !foundPressN {
		t.Fatalf("empty state / Press n copy missing from 80x24 frame:\n%s", view)
	}
	if !foundPlaceholder {
		t.Fatalf("preview placeholder missing from 80x24 frame:\n%s", view)
	}
	if strings.Contains(view, "││") {
		t.Fatalf("80x24 frame contains a doubled seam:\n%s", view)
	}
}
