package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

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

// pinStatusGlyph is the status glyph pinMarkerTestModel's rows (both
// `idle`) render in each glyph mode: SPEC §11's `○`, ASCII `o`.
func pinStatusGlyph(ascii bool) string {
	if ascii {
		return "o"
	}
	return "\u25cb"
}

// TestPinnedRowMarkerRendersBeforeName proves R160's rendering contract
// (task 006, SPEC §11's row-layout bullet and its pin bullet): line 1's
// fixed order is gutter, status glyph, pin marker, name -- so on a pinned
// row the marker sits BETWEEN the status glyph and the name, immediately
// before the name, in both glyph modes -- and an unpinned row shows no
// marker glyph and reserves no column for one: its status glyph is
// immediately followed by its name.
func TestPinnedRowMarkerRendersBeforeName(t *testing.T) {
	for _, mode := range pinGlyphModes {
		t.Run(mode.name, func(t *testing.T) {
			m := pinMarkerTestModel(t, mode.ascii)
			view := m.View()
			term := renderSettingsToEmulator(t, view, m.width, m.height)
			layout := m.computeLayout()
			sw := layout.Sidebar.Width
			status := pinStatusGlyph(mode.ascii)
			cellText := func(col, row int) string {
				if cell := term.CellAt(col, row); cell != nil {
					return cell.Content
				}
				return ""
			}

			pinnedRow := findRowContainingInSidebar(t, term, sw, "pinned-one")
			nameCol := findCol(t, term, pinnedRow, "pinned-one")
			if nameCol < 4 {
				t.Fatalf("pinned row: name at col %d leaves no room for status glyph + marker before it", nameCol)
			}
			// <status> <marker> <name>: four cells before the name.
			if got := cellText(nameCol-1, pinnedRow); got != " " {
				t.Fatalf("pinned row: col %d between marker and name = %q, want one separator space:\n%s", nameCol-1, got, view)
			}
			if got := cellText(nameCol-2, pinnedRow); got != mode.want {
				t.Fatalf("pinned row: col %d (immediately before the name) = %q, want pin marker %q:\n%s", nameCol-2, got, mode.want, view)
			}
			if got := cellText(nameCol-3, pinnedRow); got != " " {
				t.Fatalf("pinned row: col %d between status glyph and marker = %q, want one separator space:\n%s", nameCol-3, got, view)
			}
			if got := cellText(nameCol-4, pinnedRow); got != status {
				t.Fatalf("pinned row: col %d (before the marker) = %q, want the idle status glyph %q -- the marker must sit between the status glyph and the name:\n%s", nameCol-4, got, status, view)
			}
			// Exactly one marker glyph on the row, and no wrong-mode glyph.
			for c := 0; c < sw; c++ {
				got := cellText(c, pinnedRow)
				if got == mode.notWant {
					t.Fatalf("pinned row: col %d carries the wrong-mode marker %q:\n%s", c, got, view)
				}
				if got == mode.want && c != nameCol-2 {
					t.Fatalf("pinned row: a second marker %q at col %d:\n%s", got, c, view)
				}
			}

			plainRow := findRowContainingInSidebar(t, term, sw, "plain-one")
			plainNameCol := findCol(t, term, plainRow, "plain-one")
			for c := 0; c < sw; c++ {
				if got := cellText(c, plainRow); got == "\u2726" || got == "*" {
					t.Fatalf("unpinned row: col %d carries a pin-marker glyph %q:\n%s", c, got, view)
				}
			}
			// No reserved column: the unpinned row's status glyph is
			// followed directly by one space and the name.
			if got := cellText(plainNameCol-1, plainRow); got != " " {
				t.Fatalf("unpinned row: col %d before the name = %q, want one separator space:\n%s", plainNameCol-1, got, view)
			}
			if got := cellText(plainNameCol-2, plainRow); got != status {
				t.Fatalf("unpinned row: col %d = %q, want the status glyph %q directly before the name (no reserved marker column):\n%s", plainNameCol-2, got, status, view)
			}
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
// R160's rendering contract: the name truncation budget accounts for the
// marker, so a long pinned name still shows its status glyph and marker,
// is truncated (ellipsised), and no sidebar line exceeds the SIDEBAR's own
// width -- in both glyph modes, at 80x24.
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
			vbar := "\u2502"
			ellipsis := "\u2026"
			if mode.ascii {
				vbar = "|"
				ellipsis = "..."
			}

			// Every content line of the frame: the sidebar cell -- the
			// text between the left border and the sidebar/preview
			// divider -- is exactly the sidebar's content width: sw
			// columns from the left border to the divider, which sits in
			// column sw, so sw-1 between them. A row whose text overflowed the
			// sidebar would push the divider right and widen this cell.
			lines := splitLines(stripANSI(view))
			checked := 0
			var pinnedCell string
			for i, line := range lines {
				if !strings.HasPrefix(line, vbar) {
					continue
				}
				cells := strings.Split(line, vbar)
				if len(cells) < 3 {
					t.Fatalf("frame line %d has no sidebar/preview divider: %q", i, line)
				}
				if w := stringWidth(cells[1]); w != sw-1 {
					t.Fatalf("frame line %d: sidebar cell is %d columns, want exactly %d (sidebar width %d minus its left border): %q", i, w, sw-1, sw, line)
				}
				checked++
				if strings.Contains(cells[1], mode.want+" ") {
					pinnedCell = cells[1]
				}
			}
			if checked == 0 {
				t.Fatalf("no sidebar content line found in the frame:\n%s", view)
			}
			if pinnedCell == "" {
				t.Fatalf("no sidebar line carries the pin marker %q for the long-named pinned session:\n%s", mode.want, view)
			}
			// Status glyph, marker, then the TRUNCATED name.
			wantLead := pinStatusGlyph(mode.ascii) + " " + mode.want + " " + longName[:10]
			if !strings.Contains(pinnedCell, wantLead) {
				t.Fatalf("pinned row %q does not read status glyph, marker, then the name (%q)", pinnedCell, wantLead)
			}
			if !strings.HasSuffix(strings.TrimRight(pinnedCell, " "), ellipsis) {
				t.Fatalf("pinned row %q is not ellipsised with %q", pinnedCell, ellipsis)
			}
			if strings.Contains(view, longName) {
				t.Fatalf("long pinned name rendered in full (not truncated) at 80x24:\n%s", view)
			}

			// The emulator agrees: the divider sits in column sw on every
			// content row, never displaced by the pinned row's text.
			term := renderSettingsToEmulator(t, view, m.width, m.height)
			for row := 1; row < layout.Sidebar.Height-1; row++ {
				if cell := term.CellAt(sw, row); cell == nil || cell.Content != vbar {
					t.Fatalf("row %d col %d = %v, want the sidebar divider %q (sidebar text overflowed its width %d):\n%s", row, sw, cell, vbar, sw, view)
				}
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

// TestSidebarRowLeadGlyphsAreOneColumnNeverWide pins SPEC §11's EAW rule
// for the glyphs line 1 now leads with -- every status glyph and the pin
// marker: none may be East-Asian Wide/Fullwidth (one cell in every
// terminal, measured with runewidth's non-East-Asian condition, where only
// W/F runes are two cells), and every DECK_ASCII fallback is exactly one
// pure-ASCII column.
func TestSidebarRowLeadGlyphsAreOneColumnNeverWide(t *testing.T) {
	uni := New(nil, config.Settings{}, "")
	asc := New(nil, config.Settings{ASCII: true}, "")
	cond := runewidth.NewCondition()
	cond.EastAsianWidth = false
	statuses := []string{"unknown-status"}
	for _, st := range theme.StatusTokens {
		statuses = append(statuses, string(st))
	}
	type pair struct{ label, unicode, ascii string }
	var glyphs []pair
	for _, st := range statuses {
		glyphs = append(glyphs, pair{"status " + st, uni.sidebarStatusGlyph(st), asc.sidebarStatusGlyph(st)})
	}
	pinned := store.Session{ID: "p", Name: "n", Status: "idle", PinnedAt: 1}
	uniLine, _, _ := uni.sidebarRowLines(0, pinned, false)
	ascLine, _, _ := asc.sidebarRowLines(0, pinned, false)
	glyphs = append(glyphs, pair{"pin marker", strings.Fields(stripANSI(uniLine[0]))[1], strings.Fields(stripANSI(ascLine[0]))[1]})
	for _, g := range glyphs {
		if g.label == "pin marker" && (g.unicode != "\u2726" || g.ascii != "*") {
			t.Fatalf("pin marker renders %q / %q, want \u2726 / *", g.unicode, g.ascii)
		}
		if n := utf8.RuneCountInString(g.unicode); n != 1 {
			t.Fatalf("%s: unicode glyph %q is %d runes, want one", g.label, g.unicode, n)
		}
		if w := cond.StringWidth(g.unicode); w != 1 {
			t.Fatalf("%s: unicode glyph %q is East-Asian Wide/Fullwidth (width %d)", g.label, g.unicode, w)
		}
		if len(g.ascii) != 1 || g.ascii[0] >= 0x80 || g.ascii[0] <= 0x20 {
			t.Fatalf("%s: ASCII fallback %q is not one printable ASCII column", g.label, g.ascii)
		}
	}
}
