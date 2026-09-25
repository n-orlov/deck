package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 004 (A.4, R142, GH #40, SPEC §11: "A background
// reload, re-sort or re-group while the list is drifted keeps the
// wheel's offset (clamped to the new length) and does not snap back to
// the selection, which still follows its session by identity off
// screen."). Both tests below fail against the pre-fix tree (26cdbfd,
// see artifacts/probes/): scrollSidebar armed no drift flag at all, so
// sessionsLoaded's/resortSessionsLive's own unconditional
// followSelectionViewport/scrollSessionIntoView calls snapped the
// viewport straight back onto the selection on the very next reload or
// re-sort, undoing the wheel's own scroll after a single tick.

// wheelDriftReloadTestModel builds n sessions, all in one real group, tall
// enough that a wheel notch can actually move sidebarScroll away from 0,
// with the selection pinned at the FIRST row -- so a background reload's
// own selection-follow (were it still armed) would visibly snap
// sidebarScroll back toward 0, the exact behaviour a drift must suppress.
func wheelDriftReloadTestModel(n, height int) Model {
	grpID := int64(1)
	var sessions []store.Session
	for i := 0; i < n; i++ {
		sessions = append(sessions, store.Session{
			ID:        fmt.Sprintf("s%02d", i),
			Name:      fmt.Sprintf("s%02d", i),
			CWD:       "/work/grp",
			GroupName: "grp",
			GroupID:   &grpID,
			Status:    "idle",
		})
	}
	m := New(nil, config.Settings{Mouse: true}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	m.selected = rowCursor(0)
	m.selectedByUser = true
	return m
}

// TestR142WheelDriftSurvivesSuccessiveReloads is the unit half of task
// 004's success criterion 1: scrollSidebar arms a drift flag, and the
// drift keeps m.sidebarScroll unchanged (clamped to the list length)
// across at least 3 successive sessionsLoaded messages while drifted,
// with the selection still tracking its session by id.
func TestR142WheelDriftSurvivesSuccessiveReloads(t *testing.T) {
	m := wheelDriftReloadTestModel(30, 24)
	selectedID := m.sessions[0].ID

	updated, _ := m.Update(wheelDown(10, 5))
	m = updated.(Model)
	if !m.sidebarScrollDrifted {
		t.Fatalf("wheel down over the sidebar did not arm the drift flag")
	}
	driftedScroll := m.sidebarScroll
	if driftedScroll == 0 {
		t.Fatalf("fixture: wheel down did not move sidebarScroll at all")
	}

	for i := 1; i <= 3; i++ {
		reloaded := append([]store.Session(nil), m.baseSessions...)
		next, _ := m.Update(sessionsLoaded{sessions: reloaded, groups: []store.Group{{ID: 1, Name: "grp"}}})
		m = next.(Model)
		if !m.sidebarScrollDrifted {
			t.Fatalf("reload #%d cleared the drift flag", i)
		}
		if m.sidebarScroll != driftedScroll {
			t.Fatalf("reload #%d moved a drifted sidebarScroll: %d -> %d", i, driftedScroll, m.sidebarScroll)
		}
		idx, ok := m.selected.SessionIndex()
		if !ok || idx < 0 || idx >= len(m.sessions) || m.sessions[idx].ID != selectedID {
			t.Fatalf("reload #%d lost the selection by id: cursor %+v", i, m.selected)
		}
	}
}

// TestR142WheelDriftSurvivesSuccessiveReloadsClampsToShorterList proves
// the "clamped to the new list length" half of the same criterion: a
// reload that shrinks the sidebar's own entry count must pull a
// now-out-of-range drifted offset back to the new maximum, never leave
// it pointing past the end, while still not snapping it all the way back
// to the (still off-screen) selection.
func TestR142WheelDriftSurvivesSuccessiveReloadsClampsToShorterList(t *testing.T) {
	m := wheelDriftReloadTestModel(30, 24)
	selectedID := m.sessions[0].ID

	// Scroll all the way to the bottom of the full 30-session list.
	var updated Model
	for i := 0; i < 40; i++ {
		next, _ := m.Update(wheelDown(10, 5))
		updated = next.(Model)
		m = updated
	}
	if !m.sidebarScrollDrifted {
		t.Fatalf("fixture: wheel down did not arm the drift flag")
	}
	fullScroll := m.sidebarScroll
	if fullScroll == 0 {
		t.Fatalf("fixture: wheel down did not move sidebarScroll at all")
	}

	shorter := append([]store.Session(nil), m.baseSessions[:10]...)
	next, _ := m.Update(sessionsLoaded{sessions: shorter, groups: []store.Group{{ID: 1, Name: "grp"}}})
	m = next.(Model)
	if !m.sidebarScrollDrifted {
		t.Fatalf("reload cleared the drift flag")
	}
	if m.sidebarScroll >= fullScroll {
		t.Fatalf("reload to a shorter list did not clamp sidebarScroll down from %d: got %d", fullScroll, m.sidebarScroll)
	}
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	maxScroll := clampSidebarScroll(1<<30, len(m.sidebarEntries(contentWidth)), contentHeight)
	if m.sidebarScroll != maxScroll {
		t.Fatalf("reload left sidebarScroll = %d, want the clamped maximum %d", m.sidebarScroll, maxScroll)
	}
	idx, ok := m.selected.SessionIndex()
	if !ok || idx < 0 || idx >= len(m.sessions) || m.sessions[idx].ID != selectedID {
		t.Fatalf("reload to a shorter list lost the selection by id: cursor %+v", m.selected)
	}
}

// TestR142NoDriftReloadStillFollowsSelection is the negative half of
// success criterion 3: with no wheel drift in force, sessionsLoaded must
// still call followSelectionViewport exactly as it always has -- a
// selection whose rendered position moves must stay in view.
func TestR142NoDriftReloadStillFollowsSelection(t *testing.T) {
	m := wheelDriftReloadTestModel(30, 24)
	if m.sidebarScrollDrifted {
		t.Fatalf("fixture: drift flag armed with no wheel event")
	}
	m.selected = rowCursor(29)
	m.followSelectionViewport()
	if m.sidebarScroll == 0 {
		t.Fatalf("fixture: selecting the last row left sidebarScroll at 0")
	}

	m.selected = rowCursor(0)
	reloaded := append([]store.Session(nil), m.baseSessions...)
	next, _ := m.Update(sessionsLoaded{sessions: reloaded, groups: []store.Group{{ID: 1, Name: "grp"}}})
	got := next.(Model)
	assertSelectionInView(t, got, "reload with no drift in force")
}

// TestR142WheelDriftSurvivesLiveReSort is the unit half of success
// criterion 2: a drifted re-sort keeps the drift and the same selected
// session. The fixture's session names are seeded in DESCENDING order so
// switching m.settings.SortOrder to SortOrderName (an ascending sort)
// reverses the whole list, guaranteeing the selected session's own
// rendered position actually moves -- exactly the case a drift must
// still survive.
func TestR142WheelDriftSurvivesLiveReSort(t *testing.T) {
	n := 30
	var sessions []store.Session
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("s%02d", n-1-i)
		sessions = append(sessions, store.Session{ID: name, Name: name, CWD: "/work/infra", Status: "idle"})
	}
	m := New(nil, config.Settings{Mouse: true}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, 24
	m.selected = rowCursor(0)
	m.selectedByUser = true
	selectedID := m.sessions[0].ID

	updated, _ := m.Update(wheelDown(10, 5))
	m = updated.(Model)
	if !m.sidebarScrollDrifted {
		t.Fatalf("wheel down over the sidebar did not arm the drift flag")
	}
	driftedScroll := m.sidebarScroll
	if driftedScroll == 0 {
		t.Fatalf("fixture: wheel down did not move sidebarScroll at all")
	}

	m.settings.SortOrder = SortOrderName
	m.resortSessionsLive()

	if !m.sidebarScrollDrifted {
		t.Fatalf("live re-sort cleared the drift flag")
	}
	if m.sidebarScroll != driftedScroll {
		t.Fatalf("live re-sort moved a drifted sidebarScroll: %d -> %d", driftedScroll, m.sidebarScroll)
	}
	idx, ok := m.selected.SessionIndex()
	if !ok || idx < 0 || idx >= len(m.sessions) {
		t.Fatalf("live re-sort left an invalid selection: %+v", m.selected)
	}
	if m.sessions[idx].ID != selectedID {
		t.Fatalf("live re-sort lost the selection by id: got %q, want %q", m.sessions[idx].ID, selectedID)
	}
	if idx == 0 {
		t.Fatalf("fixture: re-sort by name did not move the selected session's position")
	}
}
