package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionWorkspaceDefaultsToCWDBasename proves SPEC requirement 30's
// default derivation for a Session built directly (bypassing the store, as
// every internal/tui test does): an explicit Workspace is used verbatim,
// and one left at its zero value falls back to the basename of CWD, never
// to anything repo-related.
func TestSessionWorkspaceDefaultsToCWDBasename(t *testing.T) {
	cases := []struct {
		name    string
		session store.Session
		want    string
	}{
		{"defaulted", store.Session{CWD: "/home/user/work/service-a"}, "service-a"},
		{"defaulted trailing slash", store.Session{CWD: "/home/user/work/service-a/"}, "service-a"},
		{"explicit wins", store.Session{CWD: "/home/user/work/service-a", Workspace: "team-shared"}, "team-shared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionWorkspace(tc.session); got != tc.want {
				t.Fatalf("sessionWorkspace(%+v) = %q, want %q", tc.session, got, tc.want)
			}
		})
	}
}

func groupTestModel(sessions []store.Session) Model {
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	return m
}

// TestGroupSessionsBucketsByWorkspacePreservingOrder proves requirement 30's
// grouping itself: sessions land in one group per workspace, in the order
// each workspace was first seen, and never grouped by anything derived from
// a repo (two sessions here share a workspace despite different cwds under
// it, which a repo-based grouping would not do the same way).
func TestGroupSessionsBucketsByWorkspacePreservingOrder(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Workspace: "infra"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a"},
		{ID: "a2", Name: "a2", CWD: "/work/infra/subdir", Workspace: "infra"},
		{ID: "b2", Name: "b2", CWD: "/work/service-a"},
	})
	groups := m.groupSessions()
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2 (%v)", len(groups), groups)
	}
	if groups[0].Workspace != "infra" || groups[1].Workspace != "service-a" {
		t.Fatalf("group order = [%q, %q], want [infra, service-a] (first-seen order)", groups[0].Workspace, groups[1].Workspace)
	}
	if len(groups[0].Sessions) != 2 || groups[0].Sessions[0].Session.ID != "a1" || groups[0].Sessions[1].Session.ID != "a2" {
		t.Fatalf("infra group = %+v, want [a1, a2] in that order", groups[0].Sessions)
	}
	if len(groups[1].Sessions) != 2 || groups[1].Sessions[0].Session.ID != "b1" || groups[1].Sessions[1].Session.ID != "b2" {
		t.Fatalf("service-a group = %+v, want [b1, b2] in that order", groups[1].Sessions)
	}
}

// TestSidebarBodyShowsGroupHeadersAndHidesCollapsedRows proves the
// rendering half of requirement 30: every group's header line appears, and
// collapsing one hides its member rows (their session names disappear from
// the sidebar body) while the other group's rows stay visible and the
// collapsed group's own header stays put.
func TestSidebarBodyShowsGroupHeadersAndHidesCollapsedRows(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "alpha-session", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "bravo-session", CWD: "/work/service-a", Status: "idle"},
	})
	expanded := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(expanded, "infra") || !strings.Contains(expanded, "service-a") {
		t.Fatalf("expanded body missing a group header:\n%s", expanded)
	}
	if !strings.Contains(expanded, "alpha-session") || !strings.Contains(expanded, "bravo-session") {
		t.Fatalf("expanded body missing a session row:\n%s", expanded)
	}

	m.setGroupCollapsed("infra", true)
	collapsed := strings.Join(m.sidebarBodyLines(60), "\n")
	if strings.Contains(collapsed, "alpha-session") {
		t.Fatalf("collapsed group still shows its row:\n%s", collapsed)
	}
	if !strings.Contains(collapsed, "infra") {
		t.Fatalf("collapsed group's own header disappeared:\n%s", collapsed)
	}
	if !strings.Contains(collapsed, "bravo-session") {
		t.Fatalf("collapsing one group hid the other group's row:\n%s", collapsed)
	}

	m.setGroupCollapsed("infra", false)
	reexpanded := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(reexpanded, "alpha-session") {
		t.Fatalf("re-expanding did not restore the row:\n%s", reexpanded)
	}
}

// TestCollapsedGroupStaysNavigable proves the other half of requirement
// 30's own success test: collapsing the group holding the current
// selection moves it to a session that is still visible, and ↑/↓ step
// straight over a collapsed group's hidden rows instead of getting stuck
// pressing against them.
func TestCollapsedGroupStaysNavigable(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "a2", Name: "a2", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.selected = 0 // "a1", inside "infra"

	m.setGroupCollapsed("infra", true)
	if !m.isSessionVisible(m.selected) {
		t.Fatalf("selection %d landed on a hidden row after collapsing its group", m.selected)
	}
	if m.sessions[m.selected].ID != "b1" {
		t.Fatalf("selection after collapsing = %q, want the nearest visible session b1", m.sessions[m.selected].ID)
	}

	// With "infra" still collapsed, re-select a1 directly (as if a mouse
	// click on a hidden row were somehow issued) and prove ↑/↓ skip clean
	// over the whole hidden group rather than stopping on it or doing
	// nothing.
	m.selected = 0
	next, ok := m.nextVisibleSelection(m.selected)
	if !ok || m.sessions[next].ID != "b1" {
		t.Fatalf("nextVisibleSelection from a hidden a1 = (%d, %v), want b1", next, ok)
	}
	if _, ok := m.prevVisibleSelection(0); ok {
		t.Fatalf("prevVisibleSelection before any visible row should report none, got ok=true")
	}

	m.selected = 2 // "b1", the only visible row
	if _, ok := m.nextVisibleSelection(m.selected); ok {
		t.Fatalf("nextVisibleSelection past the last visible row should report none")
	}
	prev, ok := m.prevVisibleSelection(m.selected)
	if ok {
		t.Fatalf("prevVisibleSelection with only one visible row (itself excluded) should report none, got %d", prev)
	}

	m.setGroupCollapsed("infra", false)
	if !m.isSessionVisible(0) || !m.isSessionVisible(1) {
		t.Fatalf("expanding infra again did not restore visibility for its rows")
	}
}

// TestGKeyTogglesOnlySelectedRowsGroup is task 039's SPEC §11.8 keyboard
// duplicate of toggleGroupCollapse: pressing "g" on a selected row flips
// only that row's own workspace group, leaving every other group's
// collapse state exactly as it was.
func TestGKeyTogglesOnlySelectedRowsGroup(t *testing.T) {
	// Single-workspace round trip: with nothing else to move selection to,
	// setGroupCollapsed's own "fall back to 0" behaviour (internal/tui/
	// group.go's nearestVisibleSelection) keeps m.selected pointing at the
	// same row across both collapse and the following expand, so two "g"
	// presses in a row toggle the same group closed then open again.
	one := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
	})
	one.selected = 0

	updated, _ := one.Update(key("g"))
	one = updated.(Model)
	if !one.isGroupCollapsed("infra") {
		t.Fatalf("g did not collapse the selected row's only group")
	}

	updated, _ = one.Update(key("g"))
	one = updated.(Model)
	if one.isGroupCollapsed("infra") {
		t.Fatalf("a second g did not expand the group back")
	}

	// Multi-workspace: collapsing the selected row's group must never touch
	// a different, unrelated group's own collapse state (this is the case
	// setGroupCollapsed's "move selection to the nearest still-visible
	// session" fixup actually triggers, since the collapsed group's own
	// rows all become unselectable).
	two := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	two.selected = 0 // "a1", inside "infra"

	updated, _ = two.Update(key("g"))
	two = updated.(Model)
	if !two.isGroupCollapsed("infra") {
		t.Fatalf("g did not collapse the selected row's group")
	}
	if two.isGroupCollapsed("service-a") {
		t.Fatalf("g collapsed a group other than the selected row's own")
	}
	if !two.isSessionVisible(two.selected) {
		t.Fatalf("selection %d landed on a hidden row after g collapsed its group", two.selected)
	}
}

// TestGKeyNoopUnderOverlaysAndWithNoSessions proves "g" behaves like every
// other bare-letter binding: a no-op while help or the `i` detail dialog
// covers the sidebar, and a no-op (not a panic) with no session to resolve
// a group from.
func TestGKeyNoopUnderOverlaysAndWithNoSessions(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
	})
	m.selected = 0

	m.help = true
	updated, _ := m.Update(key("g"))
	m = updated.(Model)
	if m.isGroupCollapsed("infra") {
		t.Fatalf("g collapsed a group while help was open")
	}
	m.help = false

	m.detail = true
	updated, _ = m.Update(key("g"))
	m = updated.(Model)
	if m.isGroupCollapsed("infra") {
		t.Fatalf("g collapsed a group while the detail dialog was open")
	}
	m.detail = false

	empty := groupTestModel(nil)
	updated, _ = empty.Update(key("g"))
	_ = updated.(Model) // must not panic with no sessions to select from
}

// TestToggleGroupCollapseFlipsState is the direct collapse/expand unit
// test task 028's mouse header click will drive.
func TestToggleGroupCollapseFlipsState(t *testing.T) {
	m := groupTestModel([]store.Session{{ID: "a1", CWD: "/work/infra"}})
	if m.isGroupCollapsed("infra") {
		t.Fatalf("a fresh group must start expanded")
	}
	m.toggleGroupCollapse("infra")
	if !m.isGroupCollapsed("infra") {
		t.Fatalf("toggle did not collapse")
	}
	m.toggleGroupCollapse("infra")
	if m.isGroupCollapsed("infra") {
		t.Fatalf("toggle did not expand back")
	}
}
