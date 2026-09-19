package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// I-5 / SPEC requirement 35: with `[ui] group_by_workspace` false the
// sidebar renders one flat, header-free list in the existing session sort
// order (SPEC §11), and collapse state is absent rather than merely
// inert. This file proves the render shape and the §11.2 page-size/elision
// arithmetic hold in both grouping modes; task 009 (I-6) is the dedicated
// navigation-parity proof for the same on/off switch.

func flatModeTestSessions() []store.Session {
	return []store.Session{
		{ID: "a1", Name: "alpha", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "bravo", CWD: "/work/service-a", Status: "running"},
		{ID: "c1", Name: "charlie", CWD: "/work/infra", Status: "waiting"},
	}
}

// TestFlatSidebarHasNoHeaderRowsWhenGroupingOff proves the absence half of
// requirement 35: zero sidebarLineHeader entries, and the sessions stay in
// their own existing order (never re-bucketed by workspace the way
// groupSessions would put a1 and c1's shared "infra" workspace together).
func TestFlatSidebarHasNoHeaderRowsWhenGroupingOff(t *testing.T) {
	m := New(nil, config.Settings{GroupByWorkspace: false}, "")
	m.sessions = flatModeTestSessions()

	entries := m.sidebarEntries(60)
	var rowSessionIndices []int
	for _, e := range entries {
		if e.kind == sidebarLineHeader {
			t.Fatalf("flat mode (group_by_workspace=false) rendered a header entry: %+v", e)
		}
		if e.kind == sidebarLineRow {
			// sidebarRowLines emits TWO lines per session (task 012), so
			// the same sessionIndex repeats; only record it once, on its
			// first (top) line.
			if len(rowSessionIndices) == 0 || rowSessionIndices[len(rowSessionIndices)-1] != e.sessionIndex {
				rowSessionIndices = append(rowSessionIndices, e.sessionIndex)
			}
		}
	}
	want := []int{0, 1, 2}
	if len(rowSessionIndices) != len(want) {
		t.Fatalf("row session indices = %v, want %v", rowSessionIndices, want)
	}
	for i, idx := range want {
		if rowSessionIndices[i] != idx {
			t.Fatalf("row session indices = %v, want %v (flat mode must not re-bucket by workspace)", rowSessionIndices, want)
		}
	}

	// Confirm the rendered frame itself carries no header glyph/marker
	// (m.groupHeaderText's "▾ "/"▸ " prefix), not merely that the typed
	// sidebarEntry kind is absent.
	m.width, m.height = 100, 30
	view := m.View()
	if strings.Contains(view, "\u25be") || strings.Contains(view, "\u25b8") {
		t.Fatalf("flat mode's rendered frame still contains a group collapse marker glyph:\n%s", view)
	}
}

// TestFlatSidebarHasNoCollapseBookkeeping proves the other half of
// requirement 35: collapse state is absent, not inert. Pressing `c` (the
// collapse-toggle key) with grouping off must not populate
// m.collapsedGroups, and every session must remain visible regardless.
func TestFlatSidebarHasNoCollapseBookkeeping(t *testing.T) {
	m := New(nil, config.Settings{GroupByWorkspace: false}, "")
	m.sessions = flatModeTestSessions()
	m.width, m.height = 100, 30
	m.selected = 0

	if len(m.collapsedGroups) != 0 {
		t.Fatalf("collapsedGroups should start empty, got %v", m.collapsedGroups)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got := updated.(Model)

	if len(got.collapsedGroups) != 0 {
		t.Fatalf("pressing c with grouping off populated collapse bookkeeping: %v (collapse state must be ABSENT, not merely inert, in flat mode)", got.collapsedGroups)
	}
	for i := range got.sessions {
		if !got.isSessionVisible(i) {
			t.Fatalf("session %d is not visible in flat mode; collapse must never hide a row when there is no group", i)
		}
	}
	if visibleCount := len(got.visibleSessionIndices()); visibleCount != len(got.sessions) {
		t.Fatalf("visibleSessionIndices() returned %d entries, want all %d sessions visible in flat mode", visibleCount, len(got.sessions))
	}
}

// TestPageSizeMathAgreesInBothGroupingModesAt80x24 proves §11.2's
// page-size arithmetic (sidebarRowsPerPage, PgUp/PgDn's own step) is
// asserted at exactly 80x24 in both grouping modes. The formula itself
// (rows := Sidebar.Height-2; rows/2, floor 1) does not read m.sessions at
// all, so the grouped and flat numbers necessarily agree; this test pins
// that agreement rather than assuming it, and would go red if a future
// change made the formula content-dependent without updating both modes.
func TestPageSizeMathAgreesInBothGroupingModesAt80x24(t *testing.T) {
	grouped := New(nil, config.Settings{GroupByWorkspace: true}, "")
	grouped.sessions = flatModeTestSessions()
	grouped.width, grouped.height = 80, 24

	flat := New(nil, config.Settings{GroupByWorkspace: false}, "")
	flat.sessions = flatModeTestSessions()
	flat.width, flat.height = 80, 24

	layout := grouped.computeLayout()
	if layout.Effective != LayoutSideBySide {
		t.Fatalf("test setup: 80x24 Effective = %q, want side-by-side (the golden minimum frame)", layout.Effective)
	}
	wantPerPage := (layout.Sidebar.Height - 2) / 2
	if wantPerPage < 1 {
		wantPerPage = 1
	}

	if got := grouped.sidebarRowsPerPage(); got != wantPerPage {
		t.Fatalf("grouped mode sidebarRowsPerPage() at 80x24 = %d, want %d", got, wantPerPage)
	}
	if got := flat.sidebarRowsPerPage(); got != wantPerPage {
		t.Fatalf("flat mode sidebarRowsPerPage() at 80x24 = %d, want %d (must match the grouped-mode figure: the formula is header-count-independent)", got, wantPerPage)
	}
}

// TestElisionMathAgreesInBothGroupingModesAt80x24 is requirement 35's
// other named half: a session name too long for the sidebar's content
// width is ellipsis-truncated to fit exactly, in both grouping modes, at
// 80x24 with the default sidebar_width (35 total columns, 33 content
// columns after the 2-column border/pad — the same figure
// TestSidebarContentHasOneColumnPaddingBeforeSeam exercises at a wider
// terminal). This is the flat-mode counterpart of that existing grouped
// assertion, which is left unchanged.
func TestElisionMathAgreesInBothGroupingModesAt80x24(t *testing.T) {
	longName := "a session name so long it will not fit and must be elided with an ellipsis for sure"
	for _, tc := range []struct {
		name    string
		grouped bool
	}{
		{"grouped", true},
		{"flat", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{GroupByWorkspace: tc.grouped}, "")
			m.sessions = []store.Session{{Name: longName, GroupName: "ws", Status: "running"}}
			m.width, m.height = 80, 24

			layout := m.computeLayout()
			if layout.Effective != LayoutSideBySide {
				t.Fatalf("test setup: 80x24 Effective = %q, want side-by-side", layout.Effective)
			}
			contentWidth := layout.Sidebar.Width - 2
			entries := m.sidebarEntries(contentWidth)
			var rowText string
			var rowGutter string
			found := false
			for _, e := range entries {
				if e.kind == sidebarLineRow {
					rowText = e.text
					rowGutter = e.gutter
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("no session row entry rendered: %+v", entries)
			}
			// sidebarRowLines returns raw (untruncated, unpadded) text --
			// sidebarContentLine's padTrunc is the one place that actually
			// crops it to the panel's content width (task 019's doc on
			// sidebarRowLines) -- so the ellipsis assertion is against the
			// cropped line, not the raw entry.
			cropped := m.sidebarContentLine(layout.Sidebar.Width, rowGutter, rowText, theme.Token(""))
			if !strings.Contains(cropped, "\u2026") {
				t.Fatalf("%s mode: long name row %q was not ellipsis-truncated at 80x24's sidebar width %d:\n%q", tc.name, rowText, layout.Sidebar.Width, cropped)
			}
			// The cropped, padded content line must fit exactly inside
			// the panel's content width -- this is sidebarContentLine's
			// own contract (padTrunc), asserted here rather than assumed.
			if got := stringWidth(cropped); got != layout.Sidebar.Width {
				t.Fatalf("%s mode: sidebarContentLine width = %d, want %d", tc.name, got, layout.Sidebar.Width)
			}
		})
	}
}
