package tui

import (
	"testing"

	"github.com/charmbracelet/x/vt"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// gutterGlyphModes is the two rendering modes the mark glyph is specified
// in: SPEC §11.3's `✓` normally, and the documented `*` fallback under
// DECK_ASCII (config.Settings.ASCII, what m.glyph switches on). Each mode
// names the glyph that MUST appear and the one that must NOT: asserting
// both directions is what makes these tests fail for an implementation
// that hard-codes either glyph unconditionally instead of routing through
// m.glyph.
var gutterGlyphModes = []struct {
	name    string
	ascii   bool
	want    string
	notWant string
}{
	{name: "unicode", ascii: false, want: "\u2713", notWant: "*"},
	{name: "ascii", ascii: true, want: "*", notWant: "\u2713"},
}

// sidebarGutterTestModel builds a colour-enabled, side-by-side model with
// one session, at the sidebar's own minimum width (SidebarWidthFloor, task
// 008's own floor scenario) -- the tightest gutter/truncation budget the
// row can ever render at, so a colour rule that only happens to hold at a
// wider width is not what these tests prove. `ascii` sets
// config.Settings.ASCII, i.e. DECK_ASCII's own effect on m.glyph.
func sidebarGutterTestModel(t *testing.T, ascii bool) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true, ASCII: ascii}, "")
	m.width, m.height = 100, 30
	m.sidebarWidth = SidebarWidthFloor
	m.sessions = []store.Session{
		{ID: "s1", Name: "gutx", Agent: "shell", Status: "running", CreatedAt: 1000},
	}
	m.selected = -1
	return m
}

// gutterRow renders m and returns the row (its first of two physical
// lines) containing the session's own name -- exactly like
// findRowContainingInSidebar, scoped to the sidebar's own columns so the
// preview title (which also embeds the selected session's name while
// m.interactive is true) can never be mistaken for it.
func gutterRow(t *testing.T, m Model) int {
	t.Helper()
	layout := m.computeLayout()
	sw := layout.Sidebar.Width
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	return findRowContainingInSidebar(t, term, sw, "gutx")
}

// markGlyphCol returns the column of `want` within the gutter's own
// leftmost columns of line 2, and fails if `want` is absent OR if
// `notWant` (the other mode's glyph) appears there instead -- so a
// gutter that emits one glyph regardless of config.Settings.ASCII fails
// in exactly one of the two modes rather than passing both.
func markGlyphCol(t *testing.T, term *vt.Emulator, line int, want, notWant string) int {
	t.Helper()
	col := -1
	for c := 0; c < 3; c++ {
		cell := term.CellAt(c, line)
		if cell == nil {
			continue
		}
		if cell.Content == notWant {
			t.Fatalf("row %d col %d carries the wrong-mode mark glyph %q, want %q", line, c, notWant, want)
		}
		if cell.Content == want {
			col = c
		}
	}
	if col < 0 {
		t.Fatalf("no mark glyph %q found on row %d's own leftmost gutter columns", want, line)
	}
	return col
}

// TestSidebarGutterPlainRowPaintsNoBar proves the fourth (baseline) gutter
// state: a row that is neither selected nor marked carries no `>` or `\u2713`/
// `*` glyph in its own leftmost two columns on either line, and those
// columns carry no colour of their own distinct from the row's own
// background -- i.e. column 1 (the gutter's first column, just past the
// border+pad column 0) never resolves to `accent` or `badge`.
func TestSidebarGutterPlainRowPaintsNoBar(t *testing.T) {
	m := sidebarGutterTestModel(t, false)
	accentHex := tokenHex(t, m, theme.Accent)
	badgeHex := tokenHex(t, m, theme.Badge)

	row := gutterRow(t, m)
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	for _, r := range []int{row, row + 1} {
		for _, col := range []int{1, 2} {
			cell := term.CellAt(col, r)
			if cell != nil && (cell.Content == ">" || cell.Content == "\u2713" || cell.Content == "*") {
				t.Fatalf("plain row: row %d col %d unexpectedly carries glyph %q", r, col, cell.Content)
			}
			if hex, ok := cellBgHex(t, term, col, r); ok {
				if hex == accentHex {
					t.Fatalf("plain row: row %d col %d background = accent (%s), want no bar", r, col, hex)
				}
				if hex == badgeHex {
					t.Fatalf("plain row: row %d col %d background = badge (%s), want no bar", r, col, hex)
				}
			}
		}
	}
}

// TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow proves SPEC
// §11.3's selected-row gutter: the bar is `accent`, and the `>` glyph on
// line 1 is drawn with `background` as its own foreground -- both read
// per-cell off a real vt.Emulator grid, never grepped from raw escape
// bytes.
func TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow(t *testing.T) {
	m := sidebarGutterTestModel(t, false)
	m.selected = 0
	accentHex := tokenHex(t, m, theme.Accent)
	backgroundHex := tokenHex(t, m, theme.Background)

	row := gutterRow(t, m)
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	col := findCol(t, term, row, ">")
	if hex, ok := cellBgHex(t, term, col, row); !ok || hex != accentHex {
		t.Fatalf("selected row: `>` cell background = %v, %v, want accent (%s)", hex, ok, accentHex)
	}
	if hex, ok := cellFgHex(t, term, col, row); !ok || hex != backgroundHex {
		t.Fatalf("selected row: `>` cell foreground = %v, %v, want background (%s)", hex, ok, backgroundHex)
	}
	// The bar spans both gutter columns on line 1 (the arrow's own column
	// plus its trailing space), and continues onto line 2 even though
	// nothing is marked -- selection alone paints the whole two-line bar.
	for _, c := range []int{col, col + 1} {
		if hex, ok := cellBgHex(t, term, c, row); !ok || hex != accentHex {
			t.Fatalf("selected row: line1 col %d background = %v, %v, want accent", c, hex, ok)
		}
		if hex, ok := cellBgHex(t, term, c, row+1); !ok || hex != accentHex {
			t.Fatalf("selected row: line2 col %d background = %v, %v, want accent", c, hex, ok)
		}
	}
}

// TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck proves the marked,
// unselected state: the bar is `badge`, and it carries the mark glyph on
// line 2 -- `\u2713` normally and `*` under DECK_ASCII, each mode asserted
// on its OWN model (config.Settings.ASCII set explicitly) and each also
// asserting the other mode's glyph is absent, so a gutter that emits one
// glyph unconditionally fails here instead of passing either way.
func TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck(t *testing.T) {
	for _, mode := range gutterGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := sidebarGutterTestModel(t, mode.ascii)
			m.marked = map[string]bool{"s1": true}
			badgeHex := tokenHex(t, m, theme.Badge)
			backgroundHex := tokenHex(t, m, theme.Background)

			row := gutterRow(t, m)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			col := markGlyphCol(t, term, row+1, mode.want, mode.notWant)
			if hex, ok := cellBgHex(t, term, col, row+1); !ok || hex != badgeHex {
				t.Fatalf("marked row: mark glyph background = %v, %v, want badge (%s)", hex, ok, badgeHex)
			}
			if hex, ok := cellFgHex(t, term, col, row+1); !ok || hex != backgroundHex {
				t.Fatalf("marked row: mark glyph foreground = %v, %v, want background (%s)", hex, ok, backgroundHex)
			}
			// Selection is absent, so line 1's own gutter columns share
			// the SAME badge bar too (SPEC: the marker text lives in its
			// own columns of "the same bar" -- one two-line bar, one
			// colour, not badge on line 2 only).
			if hex, ok := cellBgHex(t, term, col, row); !ok || hex != badgeHex {
				t.Fatalf("marked row: line1 bar background = %v, %v, want badge (%s) -- selection absent, badge covers the whole bar", hex, ok, badgeHex)
			}
		})
	}
}

// TestSidebarGutterMarkedAndSelectedRowStaysAccent proves selection wins
// the bar's own colour when a row is both selected and marked (SPEC:
// both cues coexist, but the bar itself stays one colour) -- the bar is
// `accent`, never `badge`, while the mark glyph still shows on line 2 in
// whichever form the mode requires (`\u2713`, `*` under DECK_ASCII).
func TestSidebarGutterMarkedAndSelectedRowStaysAccent(t *testing.T) {
	for _, mode := range gutterGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := sidebarGutterTestModel(t, mode.ascii)
			m.selected = 0
			m.marked = map[string]bool{"s1": true}
			accentHex := tokenHex(t, m, theme.Accent)
			badgeHex := tokenHex(t, m, theme.Badge)
			backgroundHex := tokenHex(t, m, theme.Background)

			row := gutterRow(t, m)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)

			arrowCol := findCol(t, term, row, ">")
			if hex, ok := cellBgHex(t, term, arrowCol, row); !ok || hex != accentHex {
				t.Fatalf("marked+selected row: `>` background = %v, %v, want accent (%s)", hex, ok, accentHex)
			}

			markCol := markGlyphCol(t, term, row+1, mode.want, mode.notWant)
			if hex, ok := cellBgHex(t, term, markCol, row+1); !ok || hex != accentHex {
				t.Fatalf("marked+selected row: mark glyph background = %v, %v, want accent (%s), selection wins over badge (%s)", hex, ok, accentHex, badgeHex)
			}
			if hex, ok := cellFgHex(t, term, markCol, row+1); !ok || hex != backgroundHex {
				t.Fatalf("marked+selected row: mark glyph foreground = %v, %v, want background (%s)", hex, ok, backgroundHex)
			}
		})
	}
}
