package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// TestStackedSidebarGutter is task 046's regression coverage for the bug
// the mouse.feature wheel-scroll hang petition (REFUTED, see notes.md)
// traced to: renderStackedFrame used to hand fullBoxContentLine only a
// row's text+bg, dropping sidebarEntry.gutter on the floor entirely, so a
// selected/marked row in the below-80-column stacked layout carried NO
// `>`/mark glyph and no accent/badge bar at all -- the exact same R119
// rules task 008/009/010 already pin for the side-by-side layout
// (sidebar_gutter_color_test.go), just never wired through the OTHER
// renderer. Each subtest below is that same file's own idiom, just built
// on a LayoutStacked model and fullBoxContentLine's own border geometry
// (border col0, pad col1, gutter cols 2-3 -- see
// TestStackedSidebarSelectionBackgroundFillsFullPanelWidth's identical
// column accounting) instead of sidebarContentLine's.
func stackedGutterTestModel(t *testing.T, ascii, color bool) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: color, ASCII: ascii}, "")
	m.width, m.height = 40, 30
	m.layoutMode = LayoutStacked
	m.sessions = []store.Session{
		{ID: "s1", Name: "gutx", Agent: "shell", Status: "running", CreatedAt: 1000},
	}
	m.selected = -1
	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("test setup: Effective = %q, want %q", layout.Effective, LayoutStacked)
	}
	return m
}

func stackedGutterRow(t *testing.T, m Model) int {
	t.Helper()
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	return findRowContaining(t, term, "gutx")
}

// TestStackedSidebarGutterSelectedRowIsAccentWithBackgroundArrow mirrors
// TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow (side-by-side)
// under LayoutStacked: the bar is `accent` and the `>` glyph's own
// foreground is `background`, both read per-cell off a real vt.Emulator.
func TestStackedSidebarGutterSelectedRowIsAccentWithBackgroundArrow(t *testing.T) {
	m := stackedGutterTestModel(t, false, true)
	m.selected = 0
	accentHex := tokenHex(t, m, theme.Accent)
	backgroundHex := tokenHex(t, m, theme.Background)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "gutx")

	col := findCol(t, term, row, ">")
	if hex, ok := cellBgHex(t, term, col, row); !ok || hex != accentHex {
		t.Fatalf("stacked selected row: `>` cell background = %v, %v, want accent (%s)", hex, ok, accentHex)
	}
	if hex, ok := cellFgHex(t, term, col, row); !ok || hex != backgroundHex {
		t.Fatalf("stacked selected row: `>` cell foreground = %v, %v, want background (%s)", hex, ok, backgroundHex)
	}
	for _, c := range []int{col, col + 1} {
		if hex, ok := cellBgHex(t, term, c, row); !ok || hex != accentHex {
			t.Fatalf("stacked selected row: line1 col %d background = %v, %v, want accent", c, hex, ok)
		}
		if hex, ok := cellBgHex(t, term, c, row+1); !ok || hex != accentHex {
			t.Fatalf("stacked selected row: line2 col %d background = %v, %v, want accent", c, hex, ok)
		}
	}
}

// TestStackedSidebarGutterMarkedUnselectedRowIsBadgeWithCheck mirrors
// TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck under LayoutStacked,
// across both the unicode mark glyph and DECK_ASCII's `*` fallback.
func TestStackedSidebarGutterMarkedUnselectedRowIsBadgeWithCheck(t *testing.T) {
	for _, mode := range gutterGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := stackedGutterTestModel(t, mode.ascii, true)
			m.marked = map[string]bool{"s1": true}
			badgeHex := tokenHex(t, m, theme.Badge)
			backgroundHex := tokenHex(t, m, theme.Background)

			row := stackedGutterRow(t, m)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			col := markGlyphCol(t, term, row+1, mode.want, mode.notWant)
			if hex, ok := cellBgHex(t, term, col, row+1); !ok || hex != badgeHex {
				t.Fatalf("stacked marked row: mark glyph background = %v, %v, want badge (%s)", hex, ok, badgeHex)
			}
			if hex, ok := cellFgHex(t, term, col, row+1); !ok || hex != backgroundHex {
				t.Fatalf("stacked marked row: mark glyph foreground = %v, %v, want background (%s)", hex, ok, backgroundHex)
			}
			if hex, ok := cellBgHex(t, term, col, row); !ok || hex != badgeHex {
				t.Fatalf("stacked marked row: line1 bar background = %v, %v, want badge (%s) -- selection absent, badge covers the whole bar", hex, ok, badgeHex)
			}
		})
	}
}

// TestStackedSidebarGutterGlyphsSurviveNoColor mirrors task 010's own
// NO_COLOR claim (panel_background_rectangle.feature's second scenario)
// for the stacked layout: with Color: false, both the selection arrow and
// the mark glyph still render as PLAIN TEXT in their fixed gutter column
// -- the marker text itself must survive degrading to monochrome even
// though no background paints the bar at all in that mode.
func TestStackedSidebarGutterGlyphsSurviveNoColor(t *testing.T) {
	for _, mode := range gutterGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := stackedGutterTestModel(t, mode.ascii, false)
			m.selected = 0
			m.marked = map[string]bool{"s1": true}

			row := stackedGutterRow(t, m)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			arrowCol := findCol(t, term, row, ">")
			if _, ok := cellBgHex(t, term, arrowCol, row); ok {
				t.Fatalf("stacked selected row under NO_COLOR: `>` at col %d carries a background, want none", arrowCol)
			}

			markCol := markGlyphCol(t, term, row+1, mode.want, mode.notWant)
			if _, ok := cellBgHex(t, term, markCol, row+1); ok {
				t.Fatalf("stacked marked row under NO_COLOR: mark glyph at col %d carries a background, want none", markCol)
			}
		})
	}
}
