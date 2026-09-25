package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// Behavioural drift assertions shared by r142_wheel_drift_test.go (task
// 004) and drift_key_follow_test.go (task 005), R142/GH #40, SPEC §11.
//
// They deliberately read no internal drift field: whether a wheel drift is
// in force is decided from what the viewport DOES on the next background
// reload, the behaviour SPEC §11 actually specifies ("A background reload
// ... while the list is drifted keeps the wheel's offset ... and does not
// snap back to the selection"; a drift "ends at the next key that moves
// or acts on the selection"). Everything here uses only APIs that predate
// R142 (sessionsLoaded, sidebarScroll, followSelectionViewport's own
// layout helpers), so the R142 tests built on them reach a real
// behavioural assertion -- and fail it -- on the unfixed tree too.

// offScreenSidebarOffset returns a valid sidebarScroll offset (inside
// clampSidebarScroll's own [0, max] range) at which the selected cursor's
// whole entry span lies outside the visible window. ok is false when the
// whole sidebar fits inside the window (no offset but 0 is valid, so no
// drift could ever be observed); any other failure is a fixture error.
func offScreenSidebarOffset(t *testing.T, label string, m Model) (offset int, ok bool) {
	t.Helper()
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	start, end := cursorSpan(m, entries, m.selected)
	if start == -1 {
		t.Fatalf("%s: fixture: selected cursor %+v has no sidebar entries", label, m.selected)
	}
	maxOffset := max(0, len(entries)-contentHeight)
	switch {
	case maxOffset == 0:
		return 0, false
	case start >= contentHeight:
		return 0, true
	case end < maxOffset:
		return maxOffset, true
	}
	t.Fatalf("%s: fixture: no offset puts selection span [%d,%d] off screen (%d entries, height %d)", label, start, end, len(entries), contentHeight)
	return 0, false
}

// reloadSameSessions delivers one background sessionsLoaded carrying the
// model's own current session and group lists -- a reload that changes
// nothing but still runs the reload's viewport handling.
func reloadSameSessions(m Model) Model {
	sessions := append([]store.Session(nil), m.baseSessions...)
	groups := append([]store.Group(nil), m.allGroups...)
	next, _ := m.Update(sessionsLoaded{sessions: sessions, groups: groups})
	return next.(Model)
}

// assertDriftStillInForce proves, through behaviour alone, that a wheel
// drift is still in force on m: on a copy, the viewport is parked where
// the selection is off screen and one background reload is delivered;
// the reload must keep that offset instead of snapping back onto the
// selection (which a reload does whenever no drift is in force).
func assertDriftStillInForce(t *testing.T, label string, m Model) {
	t.Helper()
	offset, ok := offScreenSidebarOffset(t, label, m)
	if !ok {
		t.Fatalf("%s: fixture: the whole sidebar fits on screen, so no drift can be observed", label)
	}
	m.sidebarScroll = offset
	m = reloadSameSessions(m)
	if m.sidebarScroll != offset {
		t.Fatalf("%s: the next background reload moved sidebarScroll %d -> %d, snapping back onto the selection -- no drift is in force", label, offset, m.sidebarScroll)
	}
}

// assertDriftEnded is assertDriftStillInForce's converse: on a copy, with
// the viewport parked off the selection, the next background reload must
// bring the selection back into view -- i.e. no drift is holding the
// offset any more. When the key left a sidebar that fits on screen
// entirely (a fold, say), no offset can take the selection off screen,
// so there is no drift left to observe and nothing further to prove.
func assertDriftEnded(t *testing.T, label string, m Model) {
	t.Helper()
	offset, ok := offScreenSidebarOffset(t, label, m)
	if !ok {
		return
	}
	m.sidebarScroll = offset
	m = reloadSameSessions(m)
	assertSelectionInView(t, m, label+", then a background reload with the viewport off the selection")
}
