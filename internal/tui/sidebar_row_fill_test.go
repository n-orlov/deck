package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// sidebarRowFillModel builds a colour-enabled, side-by-side-layout model
// with three short-named sessions, wide enough that names never wrap or
// truncate but short enough that padTrunc's pad-fill (the very columns
// task 321/R58b fixes) is the large majority of each row's width.
func sidebarRowFillModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "z", Agent: "shell", Status: "running", CreatedAt: 1000},
		{ID: "s2", Name: "bb", Agent: "shell", Status: "running", CreatedAt: 1000},
		{ID: "s3", Name: "ccc", Agent: "shell", Status: "running", CreatedAt: 1000},
	}
	m.selected = rowCursor(-1)
	return m
}

// findRowContainingInSidebar mirrors findRowContaining but restricts the
// scan to columns [0,sw) -- with the preview title embedding the selected
// session's name while m.interactive is true (previewTitle's own doc),
// that name can ALSO appear on the shared top-border row past the seam;
// scoping the scan to the sidebar's own columns finds the session's real
// row, not that title.
func findRowContainingInSidebar(t *testing.T, term *vt.Emulator, sw int, want string) int {
	t.Helper()
	height := term.Height()
	for row := 0; row < height; row++ {
		var b strings.Builder
		for col := 0; col < sw; col++ {
			if cell := term.CellAt(col, row); cell != nil {
				b.WriteString(cell.Content)
			}
		}
		if strings.Contains(b.String(), want) {
			return row
		}
	}
	t.Fatalf("no sidebar row (cols 0..%d) contains %q", sw-1, want)
	return -1
}

// assertRowBackgroundFillsFullWidth checks, for both lines of the row
// starting at rowLine1, that every column from the first after the
// sidebar's left border (column 1) to the last before the seam (column
// sw-1, sidebarContentLine's own trailing pad column) carries wantHex --
// EXCEPT the row's own gutter columns (2 and 3, task 008/R119's own
// reserved leftmost span) when gutterHex is non-empty, which carry
// gutterHex instead. Task 009 gives a selected row's gutter its own
// `accent` bar (SPEC §11.3), which is deliberately a DIFFERENT colour
// from `selection`/`selection_idle` on that same row -- gutterHex==""
// (every caller but the two selected-row tests below) means "no
// exception", so a row with no gutter bar of its own (unselected,
// unmarked -- TestSidebarStripeBackgroundFillsFullPanelWidth's case)
// still gets the ORIGINAL uniform-width assertion, unchanged.
func assertRowBackgroundFillsFullWidth(t *testing.T, m Model, rowLine1, sw int, wantHex, label string) {
	t.Helper()
	assertRowBackgroundFillsFullWidthWithGutter(t, m, rowLine1, sw, wantHex, "", label)
}

func assertRowBackgroundFillsFullWidthWithGutter(t *testing.T, m Model, rowLine1, sw int, wantHex, gutterHex, label string) {
	t.Helper()
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	for _, row := range []int{rowLine1, rowLine1 + 1} {
		for col := 1; col <= sw-1; col++ {
			want := wantHex
			if gutterHex != "" && (col == 2 || col == 3) {
				want = gutterHex
			}
			hex, ok := cellBgHex(t, term, col, row)
			if !ok {
				t.Fatalf("%s: row %d col %d has no background at all, want %s", label, row, col, want)
			}
			if hex != want {
				t.Fatalf("%s: row %d col %d background = %s, want %s (border=col0, seam starts col%d)", label, row, col, hex, want, sw)
			}
		}
	}
}

// TestSidebarSelectionBackgroundFillsFullPanelWidth proves the SELECTED
// row's `selection` background (SPEC requirement 42) spans every column
// after the left border up to the seam on both of the row's lines, not
// merely the columns sidebarRowLines' own text happens to draw -- the
// gap this test is red against is exactly a short name's pad-fill (and
// the sidebar's flanking single-space columns) staying uncoloured.
func TestSidebarSelectionBackgroundFillsFullPanelWidth(t *testing.T) {
	m := sidebarRowFillModel(t)
	m.selected = rowCursor(0) // "z" -- the shortest name, so the pad-fill columns dominate
	selectionHex := tokenHex(t, m, theme.Selection)

	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "z")

	assertRowBackgroundFillsFullWidthWithGutter(t, m, row, sw, selectionHex, tokenHex(t, m, theme.Accent), "selected row")
}

// TestSidebarSelectionIdleBackgroundFillsFullPanelWidth mirrors the above
// for `selection_idle` (focus moved to the preview via interactive mode,
// SPEC requirement 44) -- the second of R58b's two focus-cue tokens.
func TestSidebarSelectionIdleBackgroundFillsFullPanelWidth(t *testing.T) {
	m := sidebarRowFillModel(t)
	m.selected = rowCursor(0)
	m.interactive = true
	selIdleHex := tokenHex(t, m, theme.SelectionIdle)

	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContainingInSidebar(t, term, sw, "z")

	assertRowBackgroundFillsFullWidthWithGutter(t, m, row, sw, selIdleHex, tokenHex(t, m, theme.Accent), "selection_idle row")
}

// TestSidebarStripeBackgroundFillsFullPanelWidth mirrors the above for the
// alternating surface stripe (task 084) on a row that is NOT selected --
// the third of R58b's three backgrounds.
func TestSidebarStripeBackgroundFillsFullPanelWidth(t *testing.T) {
	m := sidebarRowFillModel(t)
	m.selected = rowCursor(-1)
	surfaceHex := tokenHex(t, m, theme.Surface)

	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	// Sessions are rendered in creation order ("z","bb","ccc" all share
	// CreatedAt, so the attention/ID tie-break is stable); one of the
	// three phases must paint theme.Surface -- find it by checking each
	// row's own name column first, the way sidebar_stripe_test.go does,
	// then assert the FULL width on whichever row that is.
	for _, name := range []string{"z", "bb", "ccc"} {
		row := findRowContaining(t, term, name)
		col := findCol(t, term, row, name)
		if hex, ok := cellBgHex(t, term, col, row); ok && hex == surfaceHex {
			assertRowBackgroundFillsFullWidth(t, m, row, sw, surfaceHex, "stripe row ("+name+")")
			return
		}
	}
	t.Fatalf("no session row painted theme.Surface at all -- the stripe never rendered")
}
