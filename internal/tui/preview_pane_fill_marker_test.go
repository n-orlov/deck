package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// paneTailFgHex is a THIRD deterministic colour, distinct from paneFgHex/
// paneBgHex above and from every builtin theme token, that the fixtures
// below leave OPEN at the very end of the pane's own visible content --
// no trailing "\x1b[0m" of the pane's own -- so a cell immediately past
// the pane's last visible column can only show deck's own theme.Background
// if this task's fix (paintForeignFill's explicit reset before painting)
// actually ran; without it the cell would still carry paneTailFgHex, the
// pane's own unclosed attribute bleeding into deck's own drawn columns.
const paneTailFgHex = "#141e2c"

// paneRowOpenTail carries, in one row: a coloured cell (X, fg+bg, mirrors
// paneSGR above), a reset mid-line (Y, plain), then a THIRD colour opened
// and never reset (Z) -- exactly the "colour set, reset mid-line,
// attribute left open" fixture this task's successCriteria names. It is
// deliberately short (3 visible columns) so cropPreviewBottomLeft's fill
// branch (padTrunc's own "columns to the right of the capture") is what
// pads it out to contentWidth, never cropRow's crop-marker branch.
const paneRowOpenTail = "\x1b[38;2;10;20;30m\x1b[48;2;200;150;100mX\x1b[0mY\x1b[38;2;20;30;44mZ"

// paneRowOpenTailWide is paneRowOpenTail's own X/Y/Z prefix followed by 300
// more columns of the SAME never-reset third colour (Z repeated) -- wide
// enough to overflow any contentWidth this package's layouts compute, so
// cropRow's crop-marker branch truncates it (never mid-glyph) and appends
// cropMarker() itself. Because the trailing colour is a FOREGROUND-only
// span (no background ever opened after Y's reset), truncateToWidth's own
// background-span tracker has nothing to auto-close at the cut point --
// the attribute really is still open exactly where the marker begins,
// unlike a background span (which truncateToWidth already closes on its
// own, per SPEC §11.3, whether or not this task's own fix exists).
var paneRowOpenTailWide = paneRowOpenTail[:len(paneRowOpenTail)-1] + strings.Repeat("Z", 300)

// capturedPaneRowsWith builds a live-preview capture whose first row is
// firstRow and whose remaining contentHeight-1 rows are plain text, mirroring
// capturedPaneRows above but letting callers supply the first row's own bytes.
func capturedPaneRowsWith(firstRow string, contentHeight int) []byte {
	rows := make([]string, contentHeight)
	rows[0] = firstRow
	for i := 1; i < contentHeight; i++ {
		rows[i] = "row"
	}
	return []byte(strings.Join(rows, "\n"))
}

// capturedPaneGeometry is the handful of coordinates every test below
// needs, computed once per layout the same way the existing
// TestCapturedPane*KeepsOwnColourFrameCarriesDeckBackground tests already
// do for their own frame assertions.
type capturedPaneGeometry struct {
	contentRow      int
	firstContentCol int
	contentWidth    int
}

// newCapturedPaneModel builds a Model+geometry for either layout (stacked
// selects the below-80-column fallback, side-by-side otherwise), sized and
// populated with firstRow as cropPreviewBottomLeft's own capture exactly
// like the existing frame tests, but returning only the interior geometry
// this file's own assertions need (content row/col/width), not the full
// border geometry those other tests already cover.
func newCapturedPaneModel(t *testing.T, stacked bool, firstRow string) (Model, *vt.Emulator, capturedPaneGeometry, string) {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	if stacked {
		m.width, m.height = 60, 30
		m.layoutMode = LayoutStacked
	} else {
		m.width, m.height = 100, 30
	}
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = 0
	m.previewLive = true
	m.previewSessionID = "sess-1"

	backgroundHex := tokenHex(t, m, theme.Background)
	if backgroundHex == paneFgHex || backgroundHex == paneBgHex || backgroundHex == paneTailFgHex {
		t.Fatalf("test setup: active theme's background %s collides with a pane fixture colour", backgroundHex)
	}

	layout := m.computeLayout()
	if stacked && layout.Effective != LayoutStacked {
		t.Fatalf("test setup: frame computed as %q, want stacked", layout.Effective)
	}
	if !stacked && layout.Effective == LayoutStacked {
		t.Fatalf("test setup: frame computed as stacked, want side-by-side")
	}

	var geo capturedPaneGeometry
	if stacked {
		lh := layout.Sidebar.Height
		pw := layout.Preview.Width
		geo.contentRow = lh + 1
		geo.contentWidth = pw - 4
		geo.firstContentCol = 2
	} else {
		sw := layout.Sidebar.Width
		geo.contentRow = 1
		geo.contentWidth = layout.Preview.Width - 4
		geo.firstContentCol = sw + 2
	}
	contentHeight := 0
	if stacked {
		contentHeight = layout.Preview.Height - 2
	} else {
		contentHeight = layout.Sidebar.Height - 2
	}
	if contentHeight <= 0 || geo.contentWidth <= 0 {
		t.Fatalf("degenerate layout: contentHeight=%d contentWidth=%d", contentHeight, geo.contentWidth)
	}

	m.previewPaneWidth = geo.contentWidth
	m.previewPaneHeight = contentHeight
	m.previewBytes = capturedPaneRowsWith(firstRow, contentHeight)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	return m, term, geo, backgroundHex
}

// assertPaneCellUntouched checks (col,row) still carries the pane's own
// fg/bg exactly, never deck's background.
func assertPaneCellUntouched(t *testing.T, term *vt.Emulator, col, row int, wantFg, wantBg string, wantBgOpen bool) {
	t.Helper()
	if wantFg != "" {
		if fg, ok := cellFgHex(t, term, col, row); !ok || fg != wantFg {
			t.Fatalf("pane cell (%d,%d) foreground = %v/%v, want %s", col, row, fg, ok, wantFg)
		}
	}
	bg, ok := cellBgHex(t, term, col, row)
	if wantBgOpen {
		if !ok || bg != wantBg {
			t.Fatalf("pane cell (%d,%d) background = %v/%v, want %s", col, row, bg, ok, wantBg)
		}
	} else if ok {
		t.Fatalf("pane cell (%d,%d) background = %s, want no background at all (pane's own, never deck's)", col, row, bg)
	}
}

// assertDeckPaintedCell checks (col,row) carries deck's OWN
// theme.Background and NOTHING left over from the pane (no foreground at
// all -- canvasBackground only ever opens a background span).
func assertDeckPaintedCell(t *testing.T, term *vt.Emulator, col, row int, backgroundHex string) {
	t.Helper()
	if fg, ok := cellFgHex(t, term, col, row); ok {
		t.Fatalf("deck-painted cell (%d,%d) foreground = %s, want none (pane's own open attribute must not survive deck's explicit reset)", col, row, fg)
	}
	bg, ok := cellBgHex(t, term, col, row)
	if !ok || bg != backgroundHex {
		t.Fatalf("deck-painted cell (%d,%d) background = %v/%v, want deck's background %s", col, row, bg, ok, backgroundHex)
	}
}

// TestCapturedPaneFillPastCaptureCarriesDeckBackground is task 003/B1 part
// 2/3's own red-first proof: cropRow's padTrunc-equivalent fill, the
// columns to the right of a captured row's own bytes when that row is
// narrower than the panel, must carry deck's theme.Background separated
// from the capture by an explicit reset -- even when the capture's own
// last visible column left an attribute open (paneRowOpenTail's trailing
// Z) rather than resetting it itself. Both layouts.
func TestCapturedPaneFillPastCaptureCarriesDeckBackground(t *testing.T) {
	for _, stacked := range []bool{false, true} {
		name := "SideBySide"
		if stacked {
			name = "Stacked"
		}
		t.Run(name, func(t *testing.T) {
			m, term, geo, backgroundHex := newCapturedPaneModel(t, stacked, paneRowOpenTail)
			_ = m

			// X: the pane's own coloured cell, untouched.
			assertPaneCellUntouched(t, term, geo.firstContentCol, geo.contentRow, paneFgHex, paneBgHex, true)
			// Y: the pane's own plain cell after its own mid-line reset --
			// no background at all, still untouched.
			assertPaneCellUntouched(t, term, geo.firstContentCol+1, geo.contentRow, "", "", false)
			// Z: the pane's own cell with its OWN attribute left open --
			// still untouched (foreground survives, no background).
			assertPaneCellUntouched(t, term, geo.firstContentCol+2, geo.contentRow, paneTailFgHex, "", false)

			// The very first fill column past the capture's own 3 visible
			// columns: deck's own drawn padding, must carry deck's
			// background and nothing of the pane's own open foreground.
			assertDeckPaintedCell(t, term, geo.firstContentCol+3, geo.contentRow, backgroundHex)
			// The LAST fill column, right before the right pad/border:
			// same, proving the whole fill span is deck-painted, not just
			// its first cell.
			lastFillCol := geo.firstContentCol + geo.contentWidth - 1
			assertDeckPaintedCell(t, term, lastFillCol, geo.contentRow, backgroundHex)
		})
	}
}

// TestCapturedPaneCropMarkerCarriesDeckBackground is task 003/B1 part 3/3's
// own red-first proof: cropRow's own crop marker (cropMarker(), SPEC
// requirement 23) must carry deck's theme.Background separated from the
// capture by an explicit reset, even when the capture's own content is cut
// off (truncateToWidth) while an attribute it opened is still open right
// at the cut point (paneRowOpenTailWide's repeated, never-reset Z run).
// Both layouts.
func TestCapturedPaneCropMarkerCarriesDeckBackground(t *testing.T) {
	for _, stacked := range []bool{false, true} {
		name := "SideBySide"
		if stacked {
			name = "Stacked"
		}
		t.Run(name, func(t *testing.T) {
			m, term, geo, backgroundHex := newCapturedPaneModel(t, stacked, paneRowOpenTailWide)
			_ = m

			// X: the pane's own coloured cell, untouched.
			assertPaneCellUntouched(t, term, geo.firstContentCol, geo.contentRow, paneFgHex, paneBgHex, true)
			// Somewhere in the middle of the truncated Z run: still the
			// pane's own open attribute, never deck's -- cropRow only
			// ever truncates the capture, it never re-composes it.
			midZCol := geo.firstContentCol + geo.contentWidth/2
			assertPaneCellUntouched(t, term, midZCol, geo.contentRow, paneTailFgHex, "", false)

			// The marker itself is the row's last column (glyph width 1
			// with colour enabled/no ASCII setting): deck-drawn chrome,
			// must carry deck's background and none of the pane's own
			// open foreground that was still live one column to its left.
			markerCol := geo.firstContentCol + geo.contentWidth - 1
			assertDeckPaintedCell(t, term, markerCol, geo.contentRow, backgroundHex)
		})
	}
}
