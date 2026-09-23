package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// paneFgHex/paneBgHex are a deterministic 24-bit foreground/background pair
// the captured pane's own SGR bytes carry below -- values no builtin theme
// (empire, daylight, matrix, cobalt, parchment) resolves any of its own
// tokens to, so a match against either one can only come from the pane's
// own escape bytes surviving untouched, never from a theme token that
// happens to coincide.
const (
	paneFgHex = "#0a141e"
	paneBgHex = "#c89664"
	// paneSGR carries the coloured cell -- an explicit foreground AND an
	// explicit background, the one pair SPEC §11.3 says deck never touches
	// in any mode -- then resets and emits a SECOND, plain 'Y' with no
	// colour of its own at all. Those two cells are the two halves of
	// §11.3's rule, and they must come out DIFFERENTLY:
	//
	//	X  the agent chose both channels, so it survives byte-exact, even
	//	   though deck fitted its foreground a moment earlier while the
	//	   background was still deck's (the two SGRs arrive separately).
	//	Y  the agent expressed no preference at all, so it carries deck's
	//	   canvas pair -- an unnamed foreground has undefined contrast, and
	//	   leaving it to the terminal is what GH #24 was.
	paneSGR = "\x1b[38;2;10;20;30m\x1b[48;2;200;150;100mX\x1b[0mY"
)

// capturedPaneRows builds a live-preview capture whose first row carries
// paneSGR (reset immediately after, so padTrunc's own pad-fill past it is
// plain, uncoloured text) and whose remaining rows are plain text, sized to
// exactly contentWidth x contentHeight real pane geometry so
// cropPreviewBottomLeft never crops or pads it.
func capturedPaneRows(contentHeight int) []byte {
	rows := make([]string, contentHeight)
	rows[0] = paneSGR
	for i := 1; i < contentHeight; i++ {
		rows[i] = "row"
	}
	return []byte(strings.Join(rows, "\n"))
}

// TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground proves
// SPEC §11.3's preview-paint rule in the default (>=80 column) side-by-side
// layout: a captured cell whose colours the AGENT chose renders with those
// colours untouched, a cell the agent left at the terminal's default
// carries deck's canvas pair instead, and the border and padding columns
// flanking both -- deck's own canvas, never the pane's -- carry deck's
// `background` token, on the content row and on the panel's top/bottom
// border rows.
func TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = rowCursor(0)
	m.previewLive = true
	m.previewSessionID = "sess-1"

	backgroundHex := tokenHex(t, m, theme.Background)
	if backgroundHex == paneFgHex || backgroundHex == paneBgHex {
		t.Fatalf("test setup: active theme's background %s collides with the pane's own fixture colour", backgroundHex)
	}

	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		t.Fatalf("test setup: frame computed as stacked, want side-by-side")
	}
	sw := layout.Sidebar.Width
	contentHeight := layout.Sidebar.Height - 2
	contentWidth := layout.Preview.Width - 4
	if contentHeight <= 0 || contentWidth <= 0 {
		t.Fatalf("degenerate layout: contentHeight=%d contentWidth=%d", contentHeight, contentWidth)
	}

	m.previewPaneWidth = contentWidth
	m.previewPaneHeight = contentHeight
	m.previewBytes = capturedPaneRows(contentHeight)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	// previewContentLine's geometry: col sw = left border (the seam),
	// sw+1 = left pad, sw+2 = first content column (where paneSGR's 'X'
	// lands, row 1 -- row 0 is the shared top border), then the pad+border
	// mirror on the right.
	topRow := 0
	contentRow := 1
	bottomRow := 1 + contentHeight
	leftBorderCol := sw
	leftPadCol := sw + 1
	firstContentCol := sw + 2
	rightPadCol := sw + 2 + contentWidth
	rightBorderCol := sw + 3 + contentWidth

	fg, ok := cellFgHex(t, term, firstContentCol, contentRow)
	if !ok || fg != paneFgHex {
		t.Fatalf("captured pane cell (%d,%d) foreground = %v/%v, want %s", firstContentCol, contentRow, fg, ok, paneFgHex)
	}
	bg, ok := cellBgHex(t, term, firstContentCol, contentRow)
	if !ok || bg != paneBgHex {
		t.Fatalf("captured pane cell (%d,%d) background = %v/%v, want %s", firstContentCol, contentRow, bg, ok, paneBgHex)
	}
	// paneSGR's second char ('Y', one column after the coloured 'X') is the
	// pane's own plain text AFTER its own internal reset -- a cell the
	// agent left at the terminal's default. deck's canvas pair must be
	// showing through it, and the pane's own colours must NOT be: a Y still
	// carrying paneBgHex would mean deck's reopen ran but the pane's span
	// was never closed.
	assertCanvasPairShowsThrough(t, term, m, firstContentCol+1, contentRow, "pane's own post-reset plain text")

	frameCells := map[string][2]int{
		"left border (content row)":  {leftBorderCol, contentRow},
		"left pad (content row)":     {leftPadCol, contentRow},
		"right pad (content row)":    {rightPadCol, contentRow},
		"right border (content row)": {rightBorderCol, contentRow},
		"top border":                 {firstContentCol, topRow},
		"bottom border":              {firstContentCol, bottomRow},
	}
	for label, pos := range frameCells {
		hex, ok := cellBgHex(t, term, pos[0], pos[1])
		if !ok {
			t.Fatalf("%s (%d,%d) has no background at all, want deck's background %s", label, pos[0], pos[1], backgroundHex)
		}
		if hex != backgroundHex {
			t.Fatalf("%s (%d,%d) background = %s, want deck's background %s", label, pos[0], pos[1], hex, backgroundHex)
		}
	}
}

// TestCapturedPaneStackedKeepsOwnColourFrameCarriesDeckBackground mirrors
// the above for the below-80-column stacked fallback, whose preview panel
// composes through fullBoxPreviewContentLine rather than
// previewContentLine (renderStackedFrame's own comment names why: a
// captured pane's own SGR bytes go through repaintForeignDefaults, never
// through the scan fullBoxContentLine's sidebar loop applies to
// deck-composed text).
func TestCapturedPaneStackedKeepsOwnColourFrameCarriesDeckBackground(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 60, 30
	m.layoutMode = LayoutStacked
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = rowCursor(0)
	m.previewLive = true
	m.previewSessionID = "sess-1"

	backgroundHex := tokenHex(t, m, theme.Background)
	if backgroundHex == paneFgHex || backgroundHex == paneBgHex {
		t.Fatalf("test setup: active theme's background %s collides with the pane's own fixture colour", backgroundHex)
	}

	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("test setup: frame computed as %q, want stacked", layout.Effective)
	}
	lh := layout.Sidebar.Height
	pw := layout.Preview.Width
	contentHeight := layout.Preview.Height - 2
	contentWidth := pw - 4
	if contentHeight <= 0 || contentWidth <= 0 {
		t.Fatalf("degenerate layout: contentHeight=%d contentWidth=%d", contentHeight, contentWidth)
	}

	m.previewPaneWidth = contentWidth
	m.previewPaneHeight = contentHeight
	m.previewBytes = capturedPaneRows(contentHeight)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	// renderStackedFrame draws the sidebar block first (top + lh-2 content
	// rows + bottom), then the preview block: its own top border, content
	// rows and bottom border, all in the panel's own left-hand columns
	// (this layout has no seam -- both panels keep all four of their own
	// borders).
	previewTopRow := lh
	previewContentRow := lh + 1
	previewBottomRow := lh + 1 + contentHeight
	leftBorderCol := 0
	leftPadCol := 1
	firstContentCol := 2
	rightPadCol := 2 + contentWidth
	rightBorderCol := 3 + contentWidth

	fg, ok := cellFgHex(t, term, firstContentCol, previewContentRow)
	if !ok || fg != paneFgHex {
		t.Fatalf("captured pane cell (%d,%d) foreground = %v/%v, want %s", firstContentCol, previewContentRow, fg, ok, paneFgHex)
	}
	bg, ok := cellBgHex(t, term, firstContentCol, previewContentRow)
	if !ok || bg != paneBgHex {
		t.Fatalf("captured pane cell (%d,%d) background = %v/%v, want %s", firstContentCol, previewContentRow, bg, ok, paneBgHex)
	}
	assertCanvasPairShowsThrough(t, term, m, firstContentCol+1, previewContentRow, "pane's own post-reset plain text")

	frameCells := map[string][2]int{
		"left border (content row)":  {leftBorderCol, previewContentRow},
		"left pad (content row)":     {leftPadCol, previewContentRow},
		"right pad (content row)":    {rightPadCol, previewContentRow},
		"right border (content row)": {rightBorderCol, previewContentRow},
		"top border":                 {firstContentCol, previewTopRow},
		"bottom border":              {firstContentCol, previewBottomRow},
	}
	for label, pos := range frameCells {
		hex, ok := cellBgHex(t, term, pos[0], pos[1])
		if !ok {
			t.Fatalf("%s (%d,%d) has no background at all, want deck's background %s", label, pos[0], pos[1], backgroundHex)
		}
		if hex != backgroundHex {
			t.Fatalf("%s (%d,%d) background = %s, want deck's background %s", label, pos[0], pos[1], hex, backgroundHex)
		}
	}
}

// assertCanvasPairShowsThrough asserts that one captured cell the agent left
// at the terminal's default carries deck's canvas pair -- background AND
// foreground, since a background alone is the half-fix GH #24 was: an
// unnamed foreground's contrast against deck's background is whatever the
// user's terminal profile happens to make it.
//
// The foreground is compared against theme.Text as the theme resolves it,
// not against a literal, so a palette edit cannot leave this asserting a
// colour deck no longer paints. It tolerates the theme's own fitted value
// the same way the paint path computes it: for a default/default cell the
// pair is deck's own, so no fitting is involved and the token's colour is
// exactly what must show.
func assertCanvasPairShowsThrough(t *testing.T, term *vt.Emulator, m Model, col, row int, what string) {
	t.Helper()
	wantBg := tokenHex(t, m, theme.Background)
	wantFg := tokenHex(t, m, theme.Text)
	bg, ok := cellBgHex(t, term, col, row)
	if !ok {
		t.Fatalf("captured pane cell (%d,%d) (%s) has no background at all, want deck's canvas %s (SPEC §11.3)", col, row, what, wantBg)
	}
	if bg != wantBg {
		t.Fatalf("captured pane cell (%d,%d) (%s) background = %s, want deck's canvas %s (SPEC §11.3)", col, row, what, bg, wantBg)
	}
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("captured pane cell (%d,%d) (%s) has no foreground at all, want deck's %s -- an unnamed foreground's contrast is whatever the terminal profile makes it (GH #24)", col, row, what, wantFg)
	}
	if fg != wantFg {
		t.Fatalf("captured pane cell (%d,%d) (%s) foreground = %s, want deck's %s", col, row, what, fg, wantFg)
	}
}
