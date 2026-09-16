package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 002/B1's red-first proof that a preview frame with no
// live capture -- no session selected at all, previewBodyLines' own
// no-session-sentence branch -- paints deck's `background` token across
// EVERY preview interior cell, including the columns past the placeholder
// sentence's own text, in both the side-by-side and stacked frames, for
// every built-in theme. Before this task previewContentLine/
// fullBoxPreviewContentLine composed the interior text span (padTrunc's
// own pad-fill included) OUTSIDE canvasBackground unconditionally, on the
// (only ever half-true) theory that anything landing there might be a
// captured pane's own foreign SGR bytes that must never be repainted
// (task 006/R118) -- so with NO live capture at all, deck's own placeholder
// copy and its pad-fill both leaked the terminal's own background instead
// of deck's, across the whole interior span. This is RED against that
// prior implementation (previewContentLine/fullBoxPreviewContentLine
// ignoring previewBodyLines' new per-row provenance and always taking the
// "foreign, do not repaint" branch) and green once the deck-owned branch
// composes border+pad+text+pad+border as one canvasBackground span.

// previewInteriorNoCaptureModel builds a colour-enabled Model with no
// sessions at all (so previewBodyLines always takes its no-session-
// sentence branch, entirely deck-owned per row) on theme t, sized so
// computeLayout resolves to wantLayout.
func previewInteriorNoCaptureModel(t *testing.T, bt *theme.Theme, width, height int, wantLayout string) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true, Theme: bt}, "")
	m.width, m.height = width, height
	layout := m.computeLayout()
	if layout.Effective != wantLayout {
		t.Fatalf("theme %q: frame computed as %q, want %q", bt.Name, layout.Effective, wantLayout)
	}
	return m
}

// TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundSideBySide covers the
// >=80-column side-by-side frame: every preview content row's interior
// column (from the leading pad column, right after the seam, through to
// the trailing pad column right before the preview's own right border)
// carries the active theme's `background` token, for every built-in theme.
func TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundSideBySide(t *testing.T) {
	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			m := previewInteriorNoCaptureModel(t, bt, 100, 30, LayoutSideBySide)
			backgroundHex := tokenHex(t, m, theme.Background)

			layout := m.computeLayout()
			sw, pw := layout.Sidebar.Width, layout.Preview.Width
			contentRows := layout.Sidebar.Height - 2
			if contentRows <= 0 {
				t.Fatalf("theme %q: degenerate layout, contentRows=%d", bt.Name, contentRows)
			}

			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			// previewContentLine's own geometry (mirrored from
			// preview_pane_repaint_test.go): sw = the shared seam (the
			// preview's own left border), sw+1 = leading pad, then the
			// interior text span, then the trailing pad at
			// sw+2+contentWidth, right border past that.
			leftPadCol := sw + 1
			rightPadCol := pw + sw - 2
			firstContentRow := 1

			for row := 0; row < contentRows; row++ {
				for col := leftPadCol; col <= rightPadCol; col++ {
					hex, ok := cellBgHex(t, term, col, firstContentRow+row)
					if !ok {
						t.Fatalf("theme %q: preview interior cell (%d,%d) has no background at all, want deck's background %s", bt.Name, col, firstContentRow+row, backgroundHex)
					}
					if hex != backgroundHex {
						t.Fatalf("theme %q: preview interior cell (%d,%d) background = %s, want deck's background %s", bt.Name, col, firstContentRow+row, hex, backgroundHex)
					}
				}
			}
		})
	}
}

// TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundStacked mirrors the
// side-by-side test above for the below-80-column stacked fallback, whose
// preview panel composes through fullBoxPreviewContentLine rather than
// previewContentLine.
func TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundStacked(t *testing.T) {
	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			m := previewInteriorNoCaptureModel(t, bt, 60, 30, LayoutStacked)
			backgroundHex := tokenHex(t, m, theme.Background)

			layout := m.computeLayout()
			lh := layout.Sidebar.Height
			pw := layout.Preview.Width
			contentRows := layout.Preview.Height - 2
			if contentRows <= 0 {
				t.Fatalf("theme %q: degenerate layout, contentRows=%d", bt.Name, contentRows)
			}

			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			// renderStackedFrame draws the sidebar block first (top + its
			// own content rows + bottom), then the preview block starting
			// at row lh: its own top border, contentRows content rows,
			// then its own bottom border.
			previewFirstContentRow := lh + 1
			leftPadCol := 1
			rightPadCol := pw - 2

			for row := 0; row < contentRows; row++ {
				for col := leftPadCol; col <= rightPadCol; col++ {
					hex, ok := cellBgHex(t, term, col, previewFirstContentRow+row)
					if !ok {
						t.Fatalf("theme %q: preview interior cell (%d,%d) has no background at all, want deck's background %s", bt.Name, col, previewFirstContentRow+row, backgroundHex)
					}
					if hex != backgroundHex {
						t.Fatalf("theme %q: preview interior cell (%d,%d) background = %s, want deck's background %s", bt.Name, col, previewFirstContentRow+row, hex, backgroundHex)
					}
				}
			}
		})
	}
}
