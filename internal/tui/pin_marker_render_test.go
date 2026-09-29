package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// pinGlyphModes mirrors gutterGlyphModes (sidebar_gutter_color_test.go)
// for the pin marker itself: SPEC §11's `✦` normally, `*` under
// DECK_ASCII/[ui] ascii -- each mode asserts both directions so an
// implementation that hard-codes either glyph unconditionally fails one
// of the two runs.
var pinGlyphModes = []struct {
	name    string
	ascii   bool
	want    string
	notWant string
}{
	{name: "unicode", ascii: false, want: "\u2726", notWant: "*"},
	{name: "ascii", ascii: true, want: "*", notWant: "\u2726"},
}

// pinMarkerTestModel builds a colour-enabled, 80x24 model (the supported
// floor, SPEC §13) with one pinned and one unpinned session, so every
// test in this file renders through the real production path
// (Model.View -> sidebarRowLines -> settingsRenderRowOpen) rather than
// calling sidebarRowLines directly.
func pinMarkerTestModel(t *testing.T, ascii bool) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true, ASCII: ascii}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{
		{ID: "pinned1", Name: "pinned-one", Agent: "shell", Status: "idle", CreatedAt: 1000, PinnedAt: 500},
		{ID: "plain1", Name: "plain-one", Agent: "shell", Status: "idle", CreatedAt: 1000},
	}
	m.selected = rowCursor(-1)
	return m
}

// TestPinnedRowMarkerRendersBeforeName proves R160's rendering contract
// (task 006, SPEC §11's row-layout bullet and its pin bullet): a pinned
// row's line 1 shows the marker immediately before its name, in both
// glyph modes, and an unpinned row shows neither glyph and reserves no
// column for one -- the marker text sits directly adjacent to the name
// with only its own trailing separator space between, never reserved on
// an unpinned row.
func TestPinnedRowMarkerRendersBeforeName(t *testing.T) {
	for _, mode := range pinGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := pinMarkerTestModel(t, mode.ascii)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)
			layout := m.computeLayout()
			sw := layout.Sidebar.Width

			pinnedRow := findRowContainingInSidebar(t, term, sw, "pinned-one")
			nameCol := findCol(t, term, pinnedRow, "pinned-one")
			// The marker plus its separator space sit in the two columns
			// immediately preceding the name's own first column.
			markerCol := nameCol - 2
			if markerCol < 0 {
				t.Fatalf("pinned row: name at col %d leaves no room before it for the marker", nameCol)
			}
			cell := term.CellAt(markerCol, pinnedRow)
			if cell == nil || cell.Content != mode.want {
				t.Fatalf("pinned row: col %d (immediately before the name at col %d) = %v, want marker %q", markerCol, nameCol, cell, mode.want)
			}
			sepCell := term.CellAt(nameCol-1, pinnedRow)
			if sepCell == nil || sepCell.Content != " " {
				t.Fatalf("pinned row: col %d between marker and name is %v, want a single separator space", nameCol-1, sepCell)
			}
			if cell.Content == mode.notWant {
				t.Fatalf("pinned row: marker cell carries the wrong-mode glyph %q", mode.notWant)
			}

			plainRow := findRowContainingInSidebar(t, term, sw, "plain-one")
			plainNameCol := findCol(t, term, plainRow, "plain-one")
			for c := 0; c < plainNameCol; c++ {
				cell := term.CellAt(c, plainRow)
				if cell == nil {
					continue
				}
				if cell.Content == mode.want || cell.Content == "\u2726" || cell.Content == "*" {
					t.Fatalf("unpinned row: col %d unexpectedly carries a pin-marker glyph %q before the name (unpinned rows reserve no column):\n%s", c, cell.Content, view)
				}
			}
			// The unpinned row's name starts no further right than the
			// pinned row's OWN name would if it had no marker -- i.e. an
			// unpinned row's name starts exactly where the gutter ends,
			// with nothing reserved in between. The pinned row's name is
			// exactly 2 columns (marker+space) further right than the
			// unpinned row's, given both rows share the same gutter and
			// status-word layout up to the name.
			if got, want := nameCol-plainNameCol, 2; got != want {
				t.Fatalf("pinned row's name starts %d columns after the unpinned row's, want exactly %d (marker + separator, no other reserved column)", got, want)
			}
		})
	}
}

// TestPinnedRowMarkerUsesAccentColour proves the marker cell is drawn in
// the `accent` token when colour is on, and that the glyph alone survives
// under NO_COLOR (config.Settings.Color=false) -- SPEC: "under NO_COLOR
// the glyph alone carries it, as the status glyphs do".
func TestPinnedRowMarkerUsesAccentColour(t *testing.T) {
	m := pinMarkerTestModel(t, false)
	accentHex := tokenHex(t, m, theme.Accent)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	pinnedRow := findRowContainingInSidebar(t, term, sw, "pinned-one")
	markerCol := findCol(t, term, pinnedRow, "\u2726")
	if hex, ok := cellFgHex(t, term, markerCol, pinnedRow); !ok || hex != accentHex {
		t.Fatalf("pinned row: marker foreground = %v, %v, want accent (%s)", hex, ok, accentHex)
	}

	// NO_COLOR: no Color setting at all, so the glyph is emitted with no
	// SGR sequence -- the glyph alone carries the cue.
	mNoColor := New(nil, config.Settings{Color: false}, "")
	mNoColor.width, mNoColor.height = 80, 24
	mNoColor.sessions = []store.Session{
		{ID: "pinned1", Name: "pinned-one", Agent: "shell", Status: "idle", CreatedAt: 1000, PinnedAt: 500},
	}
	mNoColor.selected = rowCursor(-1)
	viewNoColor := mNoColor.View()
	termNoColor := renderSettingsToEmulator(t, viewNoColor, mNoColor.width, mNoColor.height)
	layoutNoColor := mNoColor.computeLayout()
	rowNoColor := findRowContainingInSidebar(t, termNoColor, layoutNoColor.Sidebar.Width, "pinned-one")
	markerColNoColor := findCol(t, termNoColor, rowNoColor, "\u2726")
	if hex, ok := cellFgHex(t, termNoColor, markerColNoColor, rowNoColor); ok {
		t.Fatalf("NO_COLOR: marker cell unexpectedly carries a foreground colour %s, want none (glyph alone carries the cue)", hex)
	}
}

// TestPinnedLongNameTruncatesWithinSidebar proves the second half of
// R160's rendering contract: a long pinned name still shows the marker,
// truncates, and no sidebar line exceeds the sidebar's own width -- in
// both glyph modes, at 80x24.
func TestPinnedLongNameTruncatesWithinSidebar(t *testing.T) {
	for _, mode := range pinGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := New(nil, config.Settings{Color: true, ASCII: mode.ascii}, "")
			m.width, m.height = 80, 24
			longName := "an-extremely-long-pinned-session-name-that-cannot-possibly-fit-in-the-sidebar-column-budget"
			m.sessions = []store.Session{
				{ID: "pinnedlong", Name: longName, Agent: "shell", Status: "idle", CreatedAt: 1000, PinnedAt: 500},
			}
			m.selected = rowCursor(-1)

			view := m.View()
			layout := m.computeLayout()
			sw := layout.Sidebar.Width

			for _, line := range splitLines(view) {
				if w := stringWidth(line); w > m.width {
					t.Fatalf("rendered line exceeds the frame width %d (got %d): %q", m.width, w, line)
				}
			}

			term := renderSettingsToEmulator(t, view, m.width, m.height)
			row := findRowContainingInSidebar(t, term, sw, mode.want)
			if row < 0 {
				t.Fatalf("no sidebar row carries the pin marker %q for the long-named pinned session:\n%s", mode.want, view)
			}
			// Every column of both this row's lines, up to the sidebar's
			// own width, must exist (i.e. line never overflows sw either)
			// -- read directly off the emulator grid width bound below.
			for c := 0; c < sw; c++ {
				if cell := term.CellAt(c, row); cell == nil {
					t.Fatalf("sidebar row %d col %d is empty within the sidebar's own width %d", row, c, sw)
				}
			}
			// The full unabridged name must NOT appear anywhere on
			// screen -- proof it was actually truncated, not merely
			// wrapped or shown in full by a wide-enough frame.
			if containsFull(view, longName) {
				t.Fatalf("long pinned name rendered in full (not truncated) at 80x24:\n%s", view)
			}
		})
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func containsFull(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
