package tui

import (
	"strings"
	"testing"

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
	// paneSGR carries the coloured cell, then resets and emits a SECOND,
	// plain 'Y' with no colour of its own at all -- the pane's own "back
	// to default" state after its internal reset. This is exactly what a
	// naive single canvasBackground span covering border+pad+TEXT+pad+
	// border would get wrong: its scan-and-reopen treats the pane's own
	// "\x1b[0m" the same as one of deck's own embedded resets and
	// reopens deck's background right after it, repainting Y with deck's
	// background instead of leaving it exactly as the pane left it (no
	// background at all) -- the "never repainted" half of R118 this task
	// is named for, distinct from the frame/padding-carries-deck's-
	// background half TestCaptured*FrameCarriesDeckBackground's other
	// assertions already cover.
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

// TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground is
// task 006's red-first proof for R118's "captured pane content is never
// repainted" in the default (>=80 column) side-by-side layout: the
// captured cell itself must render with the PANE's own foreground and
// background untouched, while the border and padding columns immediately
// flanking it -- deck's own canvas, not the pane's -- carry deck's
// `background` token, both on the content row itself and on the panel's
// top/bottom border rows.
func TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = 0
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
	// pane's own plain text AFTER its own internal reset: it must carry NO
	// background at all, not deck's -- proving the pane's own "back to
	// default" state survives untouched rather than being repainted with
	// deck's background the instant deck's own reopen-after-reset scan
	// (canvasBackground's own trap) runs over the pane's bytes.
	if hex, ok := cellBgHex(t, term, firstContentCol+1, contentRow); ok {
		t.Fatalf("captured pane cell (%d,%d) (pane's own post-reset plain text) background = %s, want no background at all (repainted with deck's canvas)", firstContentCol+1, contentRow, hex)
	}

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
// captured pane's own SGR bytes must never be scanned/repainted the way
// fullBoxContentLine's sidebar loop's deck-composed text is).
func TestCapturedPaneStackedKeepsOwnColourFrameCarriesDeckBackground(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 60, 30
	m.layoutMode = LayoutStacked
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = 0
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
	if hex, ok := cellBgHex(t, term, firstContentCol+1, previewContentRow); ok {
		t.Fatalf("captured pane cell (%d,%d) (pane's own post-reset plain text) background = %s, want no background at all (repainted with deck's canvas)", firstContentCol+1, previewContentRow, hex)
	}

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
