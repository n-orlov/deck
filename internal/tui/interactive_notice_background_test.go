package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// This file is task 002/B1's red-first proof for the interactive preview
// branch, the same class review's B1 finding named for
// cropPreviewBottomLeft (task 001): interactiveBodyLines' own
// interactiveNotRepaintedNotice line (PRD II-49) is deck's own composed
// copy, but previewBodyLines' interactive branch used to mark the WHOLE
// slice foreign (tui.go's foreignPreviewLines(len(lines))), so
// previewContentLine/fullBoxPreviewContentLine took the "live grid
// content, never repaint" branch for that deck-owned row too and it
// leaked the terminal's own background instead of theme.Background. This
// is RED against that prior implementation and green once
// interactiveBodyLines returns its own per-row provenance and
// previewBodyLines' interactive branch passes it through instead of
// blanket-marking the slice foreign.
//
// The fixture pane (newQuietSelectionPane) is a bare tmux pane running
// only `sleep 600`, so it emits nothing at all for the life of the test:
// interactiveGridIsBlank is true and the notice shows, and the grid's own
// rows below it stay genuinely blank -- exactly the case
// m.interactiveGrid.RenderRows always actually produces via the real
// grid (see fitInteractiveBodyLines' own doc for why its pad-row branch,
// unlike this notice branch, is never reached through the real grid).
// Those blank grid rows are asserted to carry deck's `surface` token --
// the ATTACHED canvas SPEC §11.3 paints under a live grid, deliberately a
// different tone from the unattached preview's `background` so "are my
// keystrokes going to the pane?" stays answerable at a glance -- while the
// notice row, deck's own composed copy, carries `background` across its
// whole span, border to border. That the two tones differ is the point,
// and this file asserts both in one frame.

// interactiveNoticeGeometry mirrors cropDecorationGeometry (crop_
// decoration_background_test.go) for the columns/rows this file's
// assertions need.
type interactiveNoticeGeometry struct {
	firstContentRow int
	firstContentCol int
	leftPadCol      int
	rightPadCol     int
	contentWidth    int
	contentHeight   int
}

func newInteractiveNoticeModel(t *testing.T, bt *theme.Theme, stacked bool) (Model, string, interactiveNoticeGeometry) {
	t.Helper()
	m := New(nil, config.Settings{Color: true, Theme: bt}, "")
	if stacked {
		m.width, m.height = 60, 30
	} else {
		m.width, m.height = 100, 30
	}

	backgroundHex := tokenHex(t, m, theme.Background)

	layout := m.computeLayout()
	if stacked && layout.Effective != LayoutStacked {
		t.Fatalf("theme %q: frame computed as %q, want stacked", bt.Name, layout.Effective)
	}
	if !stacked && layout.Effective == LayoutStacked {
		t.Fatalf("theme %q: frame computed as stacked, want side-by-side", bt.Name)
	}

	var geo interactiveNoticeGeometry
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
	if geo.contentHeight < interactiveMinInnerRows {
		t.Fatalf("theme %q: preview content height %d is below the %d-row floor", bt.Name, geo.contentHeight, interactiveMinInnerRows)
	}

	slugBase := "bgnotice"
	if stacked {
		slugBase = "bgnotice-stacked"
	}
	slug := slugBase + "-" + bt.Name
	socket := selectionTestSocket(slug)
	target, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatalf("theme %q: tmux.SessionName(%q): %v", bt.Name, slug, err)
	}
	newQuietSelectionPane(t, socket, target, 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-" + slug, Name: slug, Slug: slug, Status: "running"}}
	m.selected = 0

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("theme %q: enterInteractive returned a non-Model tea.Model", bt.Name)
	}
	if got.attachError != "" {
		t.Fatalf("theme %q: enterInteractive refused: %q", bt.Name, got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil {
		t.Fatalf("theme %q: enterInteractive did not enter interactive mode with a live grid", bt.Name)
	}
	t.Cleanup(func() { got.exitInteractive() })

	return got, backgroundHex, geo
}

func TestInteractiveNotRepaintedNoticeCarriesDeckBackground(t *testing.T) {
	for _, stacked := range []bool{false, true} {
		name := "SideBySide"
		if stacked {
			name = "Stacked"
		}
		t.Run(name, func(t *testing.T) {
			for _, bt := range theme.Builtins() {
				bt := bt
				t.Run(bt.Name, func(t *testing.T) {
					m, backgroundHex, geo := newInteractiveNoticeModel(t, bt, stacked)
					view := m.View()
					term := renderSettingsToEmulator(t, view, m.width, m.height)

					// Row offset 0 within the preview content area is
					// interactiveBodyLines' own prepended
					// interactiveNotRepaintedNotice line -- deck's own
					// composed copy, never a byte of the live grid's own
					// rendered content -- so its whole interior span must
					// carry deck's background.
					noticeRow := geo.firstContentRow
					for col := geo.leftPadCol; col <= geo.rightPadCol; col++ {
						hex, ok := cellBgHex(t, term, col, noticeRow)
						if !ok || hex != backgroundHex {
							t.Fatalf("theme %q: notice-line cell (%d,%d) background = %v/%v, want deck's background %s", bt.Name, col, noticeRow, hex, ok, backgroundHex)
						}
					}

					// Row offset 1 is one of the live grid's own rendered
					// rows (RenderRows always returns exactly
					// contentHeight rows; the notice pushed the slice one
					// row over that and fitInteractiveBodyLines' own
					// truncation dropped the LAST one, never row 0). Its
					// TEXT columns carry the ATTACHED canvas, `surface` --
					// the grid emits a full reset for every empty cell, so
					// without deck painting under it this row would show
					// the user's own terminal background and the pane
					// region would read as unthemed.
					surfaceHex := tokenHex(t, m, theme.Surface)
					if surfaceHex == backgroundHex {
						t.Fatalf("theme %q: `surface` and `background` are both %s, so this theme cannot distinguish an attached pane at all", bt.Name, surfaceHex)
					}
					gridRow := geo.firstContentRow + 1
					for col := geo.firstContentCol; col < geo.firstContentCol+geo.contentWidth; col++ {
						hex, ok := cellBgHex(t, term, col, gridRow)
						if !ok {
							t.Fatalf("theme %q: live-grid cell (%d,%d) has no background at all, want the attached canvas `surface` %s", bt.Name, col, gridRow, surfaceHex)
						}
						if hex != surfaceHex {
							t.Fatalf("theme %q: live-grid cell (%d,%d) background = %s, want the attached canvas `surface` %s (never `background` %s, which is the UNATTACHED preview's tone)", bt.Name, col, gridRow, hex, surfaceHex, backgroundHex)
						}
					}
				})
			}
		})
	}
}
