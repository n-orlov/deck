package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 322/R58c's non-vacuous evidence for the two real gaps
// its call-site audit found beyond task 321/R58b's own sidebarContentLine
// fix (recorded in full in the commit message and docs/reports/phase3e.md):
//
//  1. renderStackedFrame (panel.go's fullBoxContentLine) never got task
//     321's background-spans-the-pad-fill treatment at all -- worse than a
//     too-narrow highlight, the stacked layout painted NO background
//     whatsoever for a selected/striped sidebar row, because task 321
//     moved sidebarRowLines/settingsRenderRowOpen to compose foreground-
//     only text with no background and no closing reset, on the
//     assumption that sidebarContentLine (side-by-side mode's only caller)
//     was the one place that needed fixing.
//  2. The settings takeover's own category/field/env-entry/search-result
//     lists (settingsRenderRow -> settingsLeftContentLine/
//     settingsRightContentLine) had EXACTLY task 321's original bug,
//     never fixed: the background closed at the end of settingsRenderRow's
//     own text, before padTrunc's pad-fill and the content line's
//     flanking padding columns.
//
// Every other padTrunc/truncateToWidth call site (sidebarContentLine and
// previewContentLine -- both fed pre-cropped/pre-closed exact-width text;
// collapsedStripContentLine and the below-minimum/main-footer/settings-
// footer truncateToWidth calls -- all plain, uncoloured text; cropRow's
// two truncateToWidth calls -- already escape-honest via task 320) is
// enumerated with its own reasoning in the task 322 commit message and
// docs/reports/phase3e.md; none of those needed a code change.

// TestStackedSidebarSelectionBackgroundFillsFullPanelWidth is gap 1's
// red-first proof: with task 322's fullBoxContentLine bg wiring reverted
// (fullBoxContentLine ignoring the entry's background entirely, as it did
// before this task), NO column in a selected stacked-mode sidebar row
// carries any background at all, so this goes red both by "wrong colour"
// and by "no colour at all" -- either failure mode proves the gap.
func TestStackedSidebarSelectionBackgroundFillsFullPanelWidth(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 30
	m.layoutMode = LayoutStacked
	m.sessions = []store.Session{
		{ID: "s1", Name: "z", Agent: "shell", Status: "running", CreatedAt: 1000},
		{ID: "s2", Name: "bb", Agent: "shell", Status: "running", CreatedAt: 1000},
	}
	m.selected = rowCursor(0)
	selectionHex := tokenHex(t, m, theme.Selection)
	// accentHex (task 046): once renderStackedFrame retains and paints the
	// row's own gutter (R119, mirroring the side-by-side sidebar's
	// sidebarContentLine, sidebar_row_fill_test.go's own precedent), a
	// selected row's gutter columns (2-3, right after border+pad) carry
	// `accent` -- deliberately a DIFFERENT colour from `selection`, per
	// SPEC §11.3 -- not the row's own selection background. Before this
	// task, fullBoxContentLine dropped the gutter entirely, so every
	// column including 2-3 read as plain `selection`; that was the bug
	// this task fixes, not a property to keep asserting.
	accentHex := tokenHex(t, m, theme.Accent)

	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("test setup: Effective = %q, want %q", layout.Effective, LayoutStacked)
	}
	lw := layout.Sidebar.Width

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "z")

	// fullBoxContentLine draws its own left AND right border (unlike the
	// side-by-side sidebar, whose right edge is the seam the preview
	// draws): the background-bearing span is columns [1, lw-2], strictly
	// inside both borders, except columns 2-3 (the gutter) which carry
	// accentHex on a selected row.
	for _, r := range []int{row, row + 1} {
		for col := 1; col <= lw-2; col++ {
			want := selectionHex
			if col == 2 || col == 3 {
				want = accentHex
			}
			hex, ok := cellBgHex(t, term, col, r)
			if !ok {
				t.Fatalf("stacked selected row: row %d col %d has no background at all, want %s", r, col, want)
			}
			if hex != want {
				t.Fatalf("stacked selected row: row %d col %d background = %s, want %s", r, col, hex, want)
			}
		}
	}
}

// TestSettingsCategoryRowBackgroundFillsFullPanelWidth is gap 2's red-first
// proof for the takeover's LEFT (category) list: with task 322's
// settingsLeftContentLine bg wiring reverted, the selected category's
// highlight stops at settingsRenderRow's own text ("> UI", 4 columns)
// rather than spanning the panel's full inner width, exactly task 321's
// original sidebar bug one level removed.
func TestSettingsCategoryRowBackgroundFillsFullPanelWidth(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.settingsOpen = true
	m.settingsFocus = settingsFocusCategories
	m.settingsCategoryIndex = 1 // "UI" -- short label, so pad-fill dominates
	m.settingsEdits = config.FileConfig{}
	selTok := m.settingsSelectionToken(settingsFocusCategories)
	selHex := tokenHex(t, m, selTok)

	width, _ := m.frameSize()
	leftWidth := settingsCategoryWidth(width)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "UI")

	// settingsLeftContentLine's geometry mirrors sidebarContentLine's
	// exactly (its own doc comment says so): border col 0, background
	// span [1, leftWidth-2] (the seam at leftWidth-1 belongs to the right
	// panel, matching sidebarContentLine's own trailing-pad-before-seam
	// shape).
	for col := 1; col <= leftWidth-2; col++ {
		hex, ok := cellBgHex(t, term, col, row)
		if !ok {
			t.Fatalf("settings category row: col %d has no background at all, want %s", col, selHex)
		}
		if hex != selHex {
			t.Fatalf("settings category row: col %d background = %s, want %s", col, hex, selHex)
		}
	}
}

// TestSettingsFieldRowBackgroundFillsFullPanelWidth mirrors the above for
// the takeover's RIGHT (field) list.
func TestSettingsFieldRowBackgroundFillsFullPanelWidth(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.settingsOpen = true
	m.settingsFocus = settingsFocusFields
	m.settingsCategoryIndex = 0 // "General" category, first field selected below
	m.settingsFieldIndex = 0
	m.settingsEdits = config.FileConfig{}
	selTok := m.settingsSelectionToken(settingsFocusFields)
	selHex := tokenHex(t, m, selTok)

	categories := settingsCategories()
	if len(categories) == 0 || len(categories[0].Fields) == 0 {
		t.Fatalf("test setup: no fields in category 0")
	}
	label := settingsFieldLabel(categories[0].Fields[0])

	width, _ := m.frameSize()
	leftWidth := settingsCategoryWidth(width)
	rightWidth := width - leftWidth

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, label)

	// settingsRightContentLine draws its own border on both sides: the
	// background span is columns [leftWidth+1, leftWidth+rightWidth-2],
	// strictly inside the shared seam (col leftWidth) and the panel's own
	// right border (col leftWidth+rightWidth-1).
	for col := leftWidth + 1; col <= leftWidth+rightWidth-2; col++ {
		hex, ok := cellBgHex(t, term, col, row)
		if !ok {
			t.Fatalf("settings field row: col %d has no background at all, want %s", col, selHex)
		}
		if hex != selHex {
			t.Fatalf("settings field row: col %d background = %s, want %s", col, hex, selHex)
		}
	}
}
