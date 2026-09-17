package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 001/B1's red-first proof that deck's OWN crop
// decorations around a live capture -- cropPreviewBottomLeft's "WxH of
// realWxrealH" geometry line (panel.go's own doc comment names it) and its
// synthesized vertical blank-fill rows -- carry deck's `background` token
// when rendered through the production line builders (previewContentLine/
// fullBoxPreviewContentLine), even though previewBodyLines' live-capture
// branch used to mark the WHOLE crop slice foreign (tui.go's
// foreignPreviewLines(len(lines))), so both builders took the
// "captured pane content, never repaint" branch for deck's own composed
// rows too. This is RED against that prior implementation (the geometry
// line and blank-fill rows leak the terminal's own background instead of
// deck's) and green once cropPreviewBottomLeft returns its own per-row
// provenance and previewBodyLines' live-capture branch passes it through
// instead of blanket-marking the slice foreign.
//
// The pane's own captured rows must NOT be repainted (task 006/R118): this
// file's fixture pane is only 2 rows tall -- shorter than the preview
// content height, so cropPreviewBottomLeft must synthesize blank-fill rows
// below it -- and its real width exceeds contentWidth, so
// cropPreviewBottomLeft must synthesize the geometry line too. The first
// captured row leaves an SGR attribute open at its very last visible
// column (mirrors preview_pane_fill_marker_test.go's paneRowOpenTail), so
// this test also proves that row's own open attribute survives untouched
// -- deck's paint reaches only ITS OWN rows, never the pane's.

// cropDecorationCaptureBytes builds a two-row live-preview capture: row 0
// is paneSGR (a closed colour run, mirrors preview_pane_repaint_test.go),
// row 1 is paneRowOpenTail (a colour opened and never reset, mirrors
// preview_pane_fill_marker_test.go) -- both fixtures already defined
// package-wide with a fixed foreground/background that collides with no
// builtin theme token.
func cropDecorationCaptureBytes() []byte {
	return []byte(paneSGR + "\n" + paneRowOpenTail)
}

// cropDecorationGeometry is the handful of coordinates this file's
// assertions need for either layout, mirroring newCapturedPaneModel's own
// geometry struct in preview_pane_fill_marker_test.go but adding the row
// offsets a crop with a geometry line and blank-fill rows needs.
type cropDecorationGeometry struct {
	firstContentRow int
	firstContentCol int
	leftPadCol      int
	rightPadCol     int
	contentWidth    int
	contentHeight   int
}

// newCropDecorationModel builds a Model+geometry for either layout,
// populated with a live capture whose real width exceeds contentWidth (so
// cropPreviewBottomLeft's geometry line fires) and whose captured raw bytes
// are only 2 rows tall -- far shorter than contentHeight -- so
// cropPreviewBottomLeft's vertical blank-fill rows fire too.
func newCropDecorationModel(t *testing.T, bt *theme.Theme, stacked bool) (Model, string, cropDecorationGeometry) {
	t.Helper()
	m := New(nil, config.Settings{Color: true, Theme: bt}, "")
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
		t.Fatalf("theme %q: background %s collides with a pane fixture colour", bt.Name, backgroundHex)
	}

	layout := m.computeLayout()
	if stacked && layout.Effective != LayoutStacked {
		t.Fatalf("theme %q: frame computed as %q, want stacked", bt.Name, layout.Effective)
	}
	if !stacked && layout.Effective == LayoutStacked {
		t.Fatalf("theme %q: frame computed as stacked, want side-by-side", bt.Name)
	}

	var geo cropDecorationGeometry
	if stacked {
		lh := layout.Sidebar.Height
		pw := layout.Preview.Width
		geo.firstContentRow = lh + 1
		geo.contentWidth = pw - 4
		geo.firstContentCol = 2
		geo.leftPadCol = 1
		geo.rightPadCol = 2 + geo.contentWidth
		geo.contentHeight = layout.Preview.Height - 2
	} else {
		sw := layout.Sidebar.Width
		geo.firstContentRow = 1
		geo.contentWidth = layout.Preview.Width - 4
		geo.firstContentCol = sw + 2
		geo.leftPadCol = sw + 1
		geo.rightPadCol = sw + 2 + geo.contentWidth
		geo.contentHeight = layout.Sidebar.Height - 2
	}
	if geo.contentHeight < 5 || geo.contentWidth <= 0 {
		t.Fatalf("theme %q: degenerate layout, contentHeight=%d contentWidth=%d", bt.Name, geo.contentHeight, geo.contentWidth)
	}

	// realWidth exceeds contentWidth so the geometry line fires; realHeight
	// is declared the same as contentHeight (only width need exceed for
	// cropPreviewBottomLeft's cropped flag), but the ACTUAL captured bytes
	// are only 2 rows, far short of avail, so blank-fill rows fire too.
	m.previewPaneWidth = geo.contentWidth + 10
	m.previewPaneHeight = geo.contentHeight
	m.previewBytes = cropDecorationCaptureBytes()

	return m, backgroundHex, geo
}

func TestCropDecorationsCarryDeckBackground(t *testing.T) {
	for _, stacked := range []bool{false, true} {
		name := "SideBySide"
		if stacked {
			name = "Stacked"
		}
		t.Run(name, func(t *testing.T) {
			for _, bt := range theme.Builtins() {
				bt := bt
				t.Run(bt.Name, func(t *testing.T) {
					m, backgroundHex, geo := newCropDecorationModel(t, bt, stacked)
					view := m.View()
					term := renderSettingsToEmulator(t, view, m.width, m.height)

					// Row offset 0 within the preview content area is
					// cropPreviewBottomLeft's own geometry line
					// ("WxH of realWxrealH") -- deck's own composed text,
					// never a byte of the pane's capture.
					geomRow := geo.firstContentRow
					for col := geo.leftPadCol; col <= geo.rightPadCol; col++ {
						hex, ok := cellBgHex(t, term, col, geomRow)
						if !ok || hex != backgroundHex {
							t.Fatalf("theme %q: geometry-line cell (%d,%d) background = %v/%v, want deck's background %s", bt.Name, col, geomRow, hex, ok, backgroundHex)
						}
					}
					if fg, ok := cellFgHex(t, term, geo.firstContentCol, geomRow); ok && (fg == paneFgHex || fg == paneTailFgHex) {
						t.Fatalf("theme %q: geometry-line cell (%d,%d) foreground = %s, want no pane colour surviving on a deck-owned row", bt.Name, geo.firstContentCol, geomRow, fg)
					}

					// The LAST content row is guaranteed blank-fill: the
					// capture is only 2 rows, far short of contentHeight-1.
					blankRow := geo.firstContentRow + geo.contentHeight - 1
					for col := geo.leftPadCol; col <= geo.rightPadCol; col++ {
						hex, ok := cellBgHex(t, term, col, blankRow)
						if !ok || hex != backgroundHex {
							t.Fatalf("theme %q: blank-fill cell (%d,%d) background = %v/%v, want deck's background %s", bt.Name, col, blankRow, hex, ok, backgroundHex)
						}
					}

					// Row offset 1: the pane's own first captured row
					// (paneSGR) -- must still carry the PANE's own
					// foreground/background, untouched by deck's paint.
					captureRow0 := geo.firstContentRow + 1
					fg, ok := cellFgHex(t, term, geo.firstContentCol, captureRow0)
					if !ok || fg != paneFgHex {
						t.Fatalf("theme %q: captured cell (%d,%d) foreground = %v/%v, want pane's own %s", bt.Name, geo.firstContentCol, captureRow0, fg, ok, paneFgHex)
					}
					bg, ok := cellBgHex(t, term, geo.firstContentCol, captureRow0)
					if !ok || bg != paneBgHex {
						t.Fatalf("theme %q: captured cell (%d,%d) background = %v/%v, want pane's own %s", bt.Name, geo.firstContentCol, captureRow0, bg, ok, paneBgHex)
					}

					// Row offset 2: the pane's own second captured row
					// (paneRowOpenTail), which leaves a foreground open at
					// its own last visible column (Z, no trailing reset)
					// -- that open attribute must survive exactly as the
					// pane left it, never repainted by deck.
					captureRow1 := geo.firstContentRow + 2
					if fg, ok := cellFgHex(t, term, geo.firstContentCol+2, captureRow1); !ok || fg != paneTailFgHex {
						t.Fatalf("theme %q: captured cell (%d,%d) (pane's own open attribute) foreground = %v/%v, want pane's own %s", bt.Name, geo.firstContentCol+2, captureRow1, fg, ok, paneTailFgHex)
					}
					if hex, ok := cellBgHex(t, term, geo.firstContentCol+2, captureRow1); ok {
						t.Fatalf("theme %q: captured cell (%d,%d) (pane's own open attribute) background = %s, want no background at all (pane's own, never deck's)", bt.Name, geo.firstContentCol+2, captureRow1, hex)
					}

					// The deck-drawn fill immediately past the pane's own
					// bytes on that same captured row (past Z's own open
					// attribute) must still carry deck's background,
					// separated from the pane's open attribute by an
					// explicit reset (paintForeignFill, task 003/B1 --
					// already fixed, re-asserted here as a sanity check
					// that this task's own change did not disturb it).
					fillCol := geo.firstContentCol + 3
					if fillCol <= geo.rightPadCol-1 {
						hex, ok := cellBgHex(t, term, fillCol, captureRow1)
						if !ok || hex != backgroundHex {
							t.Fatalf("theme %q: fill cell (%d,%d) past captured row's own bytes background = %v/%v, want deck's background %s", bt.Name, fillCol, captureRow1, hex, ok, backgroundHex)
						}
					}
				})
			}
		})
	}
}
