package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionGroupKeyReadsGroupNameVerbatim proves task 008's (R128)
// rewiring of internal/tui's group-key seam: sessionGroupKey now returns
// store.Session.GroupName verbatim, with no cwd-derived fallback of any
// kind -- unlike the removed Workspace field/store.DefaultWorkspace pair,
// a session with no group recorded groups under the empty string (the
// implicit default, SPEC §11), never under a basename of its cwd.
func TestSessionGroupKeyReadsGroupNameVerbatim(t *testing.T) {
	cases := []struct {
		name    string
		session store.Session
		want    string
	}{
		{"no group recorded", store.Session{CWD: "/home/user/work/service-a"}, ""},
		{"explicit group name", store.Session{CWD: "/home/user/work/service-a", GroupName: "team-shared"}, "team-shared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionGroupKey(tc.session); got != tc.want {
				t.Fatalf("sessionGroupKey(%+v) = %q, want %q", tc.session, got, tc.want)
			}
		})
	}
}

func groupTestModel(sessions []store.Session) Model {
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	return m
}

// groupIDPtr is the shared test helper every fixture that needs an
// explicit, real store.Session.GroupID (task 013/R129 part 3 -- collapse
// state and the §11.8 header hit-test are now keyed by group id, not by
// display name) uses to get a *int64 without a local variable at every
// call site.
func groupIDPtr(id int64) *int64 {
	return &id
}

// TestGroupSessionsBucketsByGroupKeyPreservingOrder proves requirement 30's
// grouping itself: sessions land in one group per workspace, in the order
// each workspace was first seen, and never grouped by anything derived from
// a repo (two sessions here share a workspace despite different cwds under
// it, which a repo-based grouping would not do the same way).
func TestGroupSessionsBucketsByGroupKeyPreservingOrder(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", GroupName: "infra"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", GroupName: "service-a"},
		{ID: "a2", Name: "a2", CWD: "/work/infra/subdir", GroupName: "infra"},
		{ID: "b2", Name: "b2", CWD: "/work/service-a", GroupName: "service-a"},
	})
	groups := m.groupSessions()
	// cure-01-02: the structural default group now always renders
	// unfiltered even with zero members, appended after every real group
	// (still last, per groupSortsBefore) -- two real groups plus default
	// is 3, not 2; the two indices this test actually asserts against
	// (infra, service-a) are unaffected.
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3 (%v)", len(groups), groups)
	}
	if groups[0].Name != "infra" || groups[1].Name != "service-a" {
		t.Fatalf("group order = [%q, %q], want [infra, service-a] (first-seen order)", groups[0].Name, groups[1].Name)
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
	infraID, serviceID := int64(1), int64(2)
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "alpha-session", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "b1", Name: "bravo-session", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
	})
	expanded := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(expanded, "infra") || !strings.Contains(expanded, "service-a") {
		t.Fatalf("expanded body missing a group header:\n%s", expanded)
	}
	if !strings.Contains(expanded, "alpha-session") || !strings.Contains(expanded, "bravo-session") {
		t.Fatalf("expanded body missing a session row:\n%s", expanded)
	}

	m.setGroupCollapsed(infraID, true)
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

	m.setGroupCollapsed(infraID, false)
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
	infraID, serviceID := int64(1), int64(2)
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "a2", Name: "a2", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
	})
	m.selected = rowCursor(0) // "a1", inside "infra"

	// Task 012/D.1: collapsing "infra" hides a1/a2 but never infra's OWN
	// header (a header is never hidden by its own group's collapse), so
	// setGroupCollapsed's internal nearestVisibleSelection search forward
	// from a1 now lands on service-a's header -- the very next visible
	// STOP -- rather than skipping straight through to b1's row.
	m.setGroupCollapsed(infraID, true)
	if !m.isStopVisible(m.selected) {
		t.Fatalf("selection %+v landed on a hidden stop after collapsing its group", m.selected)
	}
	if want := headerCursor(serviceID); m.selected != want {
		t.Fatalf("selection after collapsing = %+v, want %+v (service-a's header, the nearest visible stop forward)", m.selected, want)
	}

	// With "infra" still collapsed, re-select a1 directly (as if a mouse
	// click on a hidden row were somehow issued) and prove ↑/↓ skip clean
	// over the whole hidden group rather than stopping on it or doing
	// nothing -- landing on service-a's header, one visual stop away.
	m.selected = rowCursor(0)
	next, ok := m.nextVisibleSelection(m.selected)
	if !ok || next != headerCursor(serviceID) {
		t.Fatalf("nextVisibleSelection from a hidden a1 = (%+v, %v), want (%+v, true)", next, ok, headerCursor(serviceID))
	}
	// prevVisibleSelection from a1 now lands on infra's OWN header --
	// still visible despite infra's rows being hidden -- rather than
	// reporting no predecessor at all.
	prev, ok := m.prevVisibleSelection(rowCursor(0))
	if !ok || prev != headerCursor(infraID) {
		t.Fatalf("prevVisibleSelection(rowCursor(0)) = (%+v, %v), want (%+v, true) (infra's own header)", prev, ok, headerCursor(infraID))
	}

	m.selected = rowCursor(2) // "b1", second-to-last visual stop
	// The implicit default group's own header (cure-01-02: always seeded
	// while unfiltered, even with no default-group member in this
	// fixture) sits right after b1, so nextVisibleSelection now reports
	// it rather than "none".
	next, ok = m.nextVisibleSelection(m.selected)
	if !ok || next != headerCursor(0) {
		t.Fatalf("nextVisibleSelection(rowCursor(2)) = (%+v, %v), want (%+v, true) (the default group's own header)", next, ok, headerCursor(0))
	}
	// service-a's own header sits immediately before b1 in visual order
	// and is never hidden, so this now reports it rather than "none".
	prev, ok = m.prevVisibleSelection(m.selected)
	if !ok || prev != headerCursor(serviceID) {
		t.Fatalf("prevVisibleSelection(rowCursor(2)) = (%+v, %v), want (%+v, true) (service-a's own header)", prev, ok, headerCursor(serviceID))
	}

	m.setGroupCollapsed(infraID, false)
	if !m.isSessionVisible(0) || !m.isSessionVisible(1) {
		t.Fatalf("expanding infra again did not restore visibility for its rows")
	}
}

// TestGKeyTogglesOnlySelectedRowsGroup is task 039's SPEC §11.8 keyboard
// duplicate of toggleGroupCollapse: pressing "c" on a selected row flips
// only that row's own group, keyed by id (task 013/R129 part 3), leaving
// every other group's collapse state exactly as it was.
//
// Task 119: this was rebound from "g" to "c" because SPEC.md:952 reserves
// g/G for top/bottom navigation, which this key previously silently
// shadowed.
func TestGKeyTogglesOnlySelectedRowsGroup(t *testing.T) {
	// Single-workspace round trip: unfiltered groupSessions() always seeds
	// the implicit default group's own header too (cure-01-02, SPEC §11:
	// "default is not a row... it always exists"), so collapsing infra --
	// its only member row's group -- pushes selection off infra entirely,
	// onto that empty default header (task 012/D.1's own
	// nearestVisibleSelection, searching forward first). This re-selects
	// infra's own header explicitly before the second "c" -- the operator
	// action a real k/j press back onto infra would produce -- so the
	// second press targets the SAME group instead of toggling the default
	// group (which has no rows to hide) by accident.
	infraID := int64(1)
	one := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
	})
	one.selected = rowCursor(0)

	updated, _ := one.Update(key("c"))
	one = updated.(Model)
	if !one.isGroupCollapsed(infraID) {
		t.Fatalf("c did not collapse the selected row's only group")
	}

	one.selected = headerCursor(infraID)
	updated, _ = one.Update(key("c"))
	one = updated.(Model)
	if one.isGroupCollapsed(infraID) {
		t.Fatalf("a second c did not expand the group back")
	}

	// Multi-workspace: collapsing the selected row's group must never touch
	// a different, unrelated group's own collapse state (this is the case
	// setGroupCollapsed's "move selection to the nearest still-visible
	// session" fixup actually triggers, since the collapsed group's own
	// rows all become unselectable).
	serviceID := int64(2)
	two := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
	})
	two.selected = rowCursor(0) // "a1", inside "infra"

	updated, _ = two.Update(key("c"))
	two = updated.(Model)
	if !two.isGroupCollapsed(infraID) {
		t.Fatalf("c did not collapse the selected row's group")
	}
	if two.isGroupCollapsed(serviceID) {
		t.Fatalf("c collapsed a group other than the selected row's own")
	}
	if !two.isStopVisible(two.selected) {
		t.Fatalf("selection %+v landed on a hidden stop after c collapsed its group", two.selected)
	}
}

// TestGKeyNoopUnderOverlaysAndWithNoSessions proves "c" behaves like every
// other bare-letter binding: a no-op while help or the `i` detail dialog
// covers the sidebar, and a no-op (not a panic) with no session to resolve
// a group from.
func TestGKeyNoopUnderOverlaysAndWithNoSessions(t *testing.T) {
	infraID := int64(1)
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
	})
	m.selected = rowCursor(0)

	m.help = true
	updated, _ := m.Update(key("c"))
	m = updated.(Model)
	if m.isGroupCollapsed(infraID) {
		t.Fatalf("c collapsed a group while help was open")
	}
	m.help = false

	m.detail = true
	updated, _ = m.Update(key("c"))
	m = updated.(Model)
	if m.isGroupCollapsed(infraID) {
		t.Fatalf("c collapsed a group while the detail dialog was open")
	}
	m.detail = false

	empty := groupTestModel(nil)
	updated, _ = empty.Update(key("c"))
	_ = updated.(Model) // must not panic with no sessions to select from
}

// TestGGKeysJumpToFirstAndLastVisibleRow is task 119's SPEC.md:952 "g/G
// top/bottom" keyboard binding: g selects the first visible STOP in
// visual order, G the last, and both skip a collapsed group's hidden
// ROWS exactly like a single ↑/↓ press would -- but never skip a header,
// which (task 012/D.1) is never itself hidden by its own group's
// collapse.
func TestGGKeysJumpToFirstAndLastVisibleRow(t *testing.T) {
	infraID, serviceID := int64(1), int64(2)
	m := groupTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "a2", Name: "a2", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
	})
	m.selected = rowCursor(1)

	updated, _ := m.Update(key("g"))
	m = updated.(Model)
	if want := headerCursor(infraID); m.selected != want {
		t.Fatalf("g did not select the first visible stop, got %+v, want %+v (infra's own header)", m.selected, want)
	}

	updated, _ = m.Update(key("G"))
	m = updated.(Model)
	// visualOrder groups by workspace; the last group's last row is the
	// overall last visible stop regardless of which workspace sorts last
	// (a header always precedes its own rows, so the very last stop is
	// always a row here).
	order := m.visualOrder()
	want := order[len(order)-1]
	if m.selected != want {
		t.Fatalf("G did not select the last visible stop: got %+v, want %+v", m.selected, want)
	}

	// Collapse the "infra" group: g must still land on infra's own
	// header -- the first visible stop -- never on a hidden a1/a2, and
	// never skipped past straight to b1 either, since a header is never
	// hidden by its own group's collapse.
	m.selected = rowCursor(2) // b1
	m.toggleGroupCollapse(infraID)
	updated, _ = m.Update(key("g"))
	m = updated.(Model)
	if want := headerCursor(infraID); m.selected != want {
		t.Fatalf("g with infra collapsed should land on infra's own header (still visible), got %+v, want %+v", m.selected, want)
	}

	// No-ops: help/detail cover the sidebar, and no sessions means nothing
	// to select.
	m.help = true
	prevSelected := m.selected
	updated, _ = m.Update(key("g"))
	m = updated.(Model)
	if m.selected != prevSelected {
		t.Fatalf("g moved selection while help was open")
	}
	m.help = false

	empty := groupTestModel(nil)
	updated, _ = empty.Update(key("g"))
	_ = updated.(Model) // must not panic with no sessions
	updated, _ = empty.Update(key("G"))
	_ = updated.(Model) // must not panic with no sessions
}

// TestToggleGroupCollapseFlipsState is the direct collapse/expand unit
// test task 028's mouse header click drives.
func TestToggleGroupCollapseFlipsState(t *testing.T) {
	infraID := int64(1)
	m := groupTestModel([]store.Session{{ID: "a1", CWD: "/work/infra", GroupName: "infra", GroupID: &infraID}})
	if m.isGroupCollapsed(infraID) {
		t.Fatalf("a fresh group must start expanded")
	}
	m.toggleGroupCollapse(infraID)
	if !m.isGroupCollapsed(infraID) {
		t.Fatalf("toggle did not collapse")
	}
	m.toggleGroupCollapse(infraID)
	if m.isGroupCollapsed(infraID) {
		t.Fatalf("toggle did not expand back")
	}
}

// TestSessionGroupIDDefaultsAndDanglingBothResolveToZero proves
// sessionGroupID's own contract (task 013/R129 part 3): a session with no
// group recorded (nil GroupID) and a session whose GroupID no longer
// resolves to a live groups row (a dangling id -- GroupName reads back
// empty either way, per SPEC §11's "renders under default rather than
// vanishing") both resolve to the SAME sentinel id, 0, never to the raw
// dangling id itself and never to two different values that would
// otherwise split one visual "default" bucket into two collapsible
// entries.
func TestSessionGroupIDDefaultsAndDanglingBothResolveToZero(t *testing.T) {
	danglingID := int64(999)
	cases := []struct {
		name    string
		session store.Session
	}{
		{"nil GroupID, empty GroupName", store.Session{ID: "a"}},
		{"dangling GroupID, resolved GroupName empty", store.Session{ID: "b", GroupID: &danglingID, GroupName: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionGroupID(tc.session); got != 0 {
				t.Fatalf("sessionGroupID(%+v) = %d, want 0", tc.session, got)
			}
		})
	}

	// The same pair, rendered together, must bucket into ONE default group
	// (GroupID 0), not two.
	m := groupTestModel([]store.Session{cases[0].session, cases[1].session})
	groups := m.groupSessions()
	if len(groups) != 1 || groups[0].Name != "" || groups[0].GroupID != 0 {
		t.Fatalf("groupSessions() = %+v, want exactly one default bucket (Name=\"\", GroupID=0)", groups)
	}
	if len(groups[0].Sessions) != 2 {
		t.Fatalf("default bucket has %d sessions, want 2", len(groups[0].Sessions))
	}
}
