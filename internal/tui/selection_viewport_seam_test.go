package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// selectionSeamTestModel builds a sidebar with n sessions split across two
// real groups (three in group "a", the rest in group "b"), so a group
// header always sits immediately above the first session of group "b" --
// the one geometry where a margin measured in raw entry lines, rather than
// in sidebarLineRow entries, silently eats half of the context row.
func selectionSeamTestModel(n, height int) Model {
	var sessions []store.Session
	for i := 0; i < n; i++ {
		group := "a"
		if i >= 3 {
			group = "b"
		}
		name := string(rune('a' + i))
		sessions = append(sessions, store.Session{ID: name, Name: name, CWD: "/work/" + group, GroupName: group})
	}
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	return m
}

// selectionSpan returns the sidebar entry-line span of the session row at
// sessionIndex, plus the entry indices of every OTHER row line before and
// after it, so a test can name the whole preceding/following session row
// (two sidebarLineRow entries) without hard-coding entry numbers.
func selectionSpan(entries []sidebarEntry, sessionIndex int) (start, end int, before, after []int) {
	start, end = -1, -1
	for i, e := range entries {
		if e.kind != sidebarLineRow {
			continue
		}
		switch {
		case e.sessionIndex == sessionIndex:
			if start == -1 {
				start = i
			}
			end = i
		case start == -1:
			before = append(before, i)
		default:
			after = append(after, i)
		}
	}
	return start, end, before, after
}

// TestSetSelectionKeepsWholeContextRowAcrossAGroupHeader is task 007/C.1's
// group-boundary probe: selecting the first session of group "b" -- whose
// own group header occupies the entry line directly above it -- must still
// keep the WHOLE preceding session row (both of its sidebarLineRow
// entries) inside the window, since the list has one to offer. A margin
// counted in raw entry lines (start-2) instead of in row entries lands on
// the header and clips the first line of that row.
func TestSetSelectionKeepsWholeContextRowAcrossAGroupHeader(t *testing.T) {
	m := selectionSeamTestModel(8, 13)
	layout := m.computeLayout()
	entries := m.sidebarEntries(sidebarEntryContentWidth(layout))
	contentHeight := layout.Sidebar.Height - 2
	if contentHeight >= len(entries) {
		t.Fatalf("fixture does not scroll: contentHeight %d >= %d entries", contentHeight, len(entries))
	}
	start, end, before, after := selectionSpan(entries, 3)
	if start == -1 {
		t.Fatalf("session 3 has no row entries in %d entries", len(entries))
	}
	if entries[start-1].kind == sidebarLineRow {
		t.Fatalf("fixture is not a group boundary: entry %d above the selection is a row, not a header", start-1)
	}
	if len(before) < 2 {
		t.Fatalf("fixture needs a whole preceding session row, got row entries %v", before)
	}
	// Approach the selection from below: the window starts exactly at the
	// selection's first line, so the seam has to scroll UP to uncover the
	// context row the criterion asks for.
	m.sidebarScroll = start
	m.setSelection(3)
	wantAtMost := before[len(before)-2] // first line of the preceding session row
	if m.sidebarScroll > wantAtMost {
		t.Fatalf("sidebarScroll = %d after selecting the first session of group b; that clips the preceding session row (entries %v), want <= %d", m.sidebarScroll, before[len(before)-2:], wantAtMost)
	}
	if m.sidebarScroll < 0 || start < m.sidebarScroll || end >= m.sidebarScroll+contentHeight {
		t.Fatalf("selection span [%d,%d] not inside window [%d,%d) (after=%v)", start, end, m.sidebarScroll, m.sidebarScroll+contentHeight, after)
	}
}

// TestSetSelectionStaysInBoundsAndFlushAtTheListEnds walks the selection
// over every session of several list sizes and asserts the seam's three
// standing invariants: the selection's own span is always fully inside the
// window, the offset never goes negative, and it never exceeds the flush
// bound clampSidebarScroll enforces (so no blank tail below the last
// entry) -- including the degenerate lists that are shorter than the
// window and therefore always sit flush at 0.
func TestSetSelectionStaysInBoundsAndFlushAtTheListEnds(t *testing.T) {
	for _, n := range []int{1, 2, 5, 8, 20} {
		m := selectionSeamTestModel(n, 13)
		layout := m.computeLayout()
		contentWidth := sidebarEntryContentWidth(layout)
		contentHeight := layout.Sidebar.Height - 2
		entries := m.sidebarEntries(contentWidth)
		maxOffset := max(0, len(entries)-contentHeight)
		for i := 0; i < n; i++ {
			m.setSelection(i)
			if m.sidebarScroll < 0 || m.sidebarScroll > maxOffset {
				t.Fatalf("n=%d selected=%d: sidebarScroll = %d, want within [0,%d]", n, i, m.sidebarScroll, maxOffset)
			}
			start, end, _, after := selectionSpan(entries, i)
			if start < m.sidebarScroll || end >= m.sidebarScroll+contentHeight {
				t.Fatalf("n=%d selected=%d: span [%d,%d] outside window [%d,%d)", n, i, start, end, m.sidebarScroll, m.sidebarScroll+contentHeight)
			}
			// Where a whole session row follows and the list can still
			// scroll, that row must be inside the window too.
			if len(after) >= 2 && m.sidebarScroll < maxOffset && after[1] >= m.sidebarScroll+contentHeight {
				t.Fatalf("n=%d selected=%d: sidebarScroll = %d clips the following session row (entries %v), window ends at %d, max offset %d", n, i, m.sidebarScroll, after[:2], m.sidebarScroll+contentHeight, maxOffset)
			}
		}
	}
}
