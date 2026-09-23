package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// This file is task 014/D.3's own dedicated coverage for `c`/left/right
// (SPEC §11.8 gap: the collapsible headers requirement 30 grants had no
// explicit fold/unfold keys, only a toggle). `c` is task 013/119's
// existing binding, exercised elsewhere (group_test.go); left and right
// are new here -- left always FOLDS, right always UNFOLDS, keyed off
// whatever group the cursor currently names (cursorGroupID: a header
// cursor's own id, or the group id of the session under a row cursor),
// never toggling.
//
// Every test below also pins task 014's setGroupCollapsed change: folding
// no longer evicts the cursor onto a DIFFERENT group's nearest visible
// stop -- a header cursor stays exactly where it is (a header is never
// hidden by its own group's collapse), and a row cursor lands on that
// SAME group's own header, never a neighbour's.

// foldUnfoldTestModel builds a model with three real, distinctly-ID'd
// groups ("a", "b", "c") plus whatever extra sessions/groups the caller
// wants layered on afterward -- distinct GroupIDs matter here (task 008's
// own gotcha: two groups told apart only by GroupName, with a nil
// store.Session.GroupID, collapse and expand TOGETHER under the shared
// sentinel 0).
func foldUnfoldTestModel() (m Model, idA, idB, idC int64) {
	idA, idB, idC = 1, 2, 3
	m = groupTestModel([]store.Session{
		{ID: "a0", Name: "a0", CWD: "/work/a", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "a1", Name: "a1", CWD: "/work/a", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "b0", Name: "b0", CWD: "/work/b", Status: "idle", GroupName: "b", GroupID: &idB},
		{ID: "c0", Name: "c0", CWD: "/work/c", Status: "idle", GroupName: "c", GroupID: &idC},
	})
	return m, idA, idB, idC
}

// TestLeftRightFoldUnfoldPopulatedGroupFromHeader proves left/right on a
// header cursor: left folds, right unfolds, and the cursor never leaves
// that header (task 014: a header is never hidden by its own collapse, so
// folding from one never needs to move it).
func TestLeftRightFoldUnfoldPopulatedGroupFromHeader(t *testing.T) {
	m, idA, _, _ := foldUnfoldTestModel()
	m.selected = headerCursor(idA)

	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(idA) {
		t.Fatalf("left did not fold group a")
	}
	if want := headerCursor(idA); m.selected != want {
		t.Fatalf("left folding from a's own header moved the cursor to %+v, want %+v (unchanged)", m.selected, want)
	}

	// left again on an already-folded group is a no-op, never a toggle.
	updated, _ = m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(idA) {
		t.Fatalf("a second left un-collapsed group a -- left must fold, never toggle")
	}

	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("right did not unfold group a")
	}
	if want := headerCursor(idA); m.selected != want {
		t.Fatalf("right unfolding from a's own header moved the cursor to %+v, want %+v (unchanged)", m.selected, want)
	}

	// right again on an already-unfolded group is a no-op too.
	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("a second right collapsed group a -- right must unfold, never toggle")
	}
}

// TestLeftRightFoldUnfoldPopulatedGroupFromRow proves left/right resolved
// from a ROW cursor: left folds that row's own group and (task 014)
// relocates the cursor onto that SAME group's own header -- never a
// neighbour's -- and right unfolds it again, leaving the cursor on the
// header where left parked it (right never moves the cursor, since
// nothing it does hides anything the cursor could be resting on).
func TestLeftRightFoldUnfoldPopulatedGroupFromRow(t *testing.T) {
	m, idA, idB, _ := foldUnfoldTestModel()
	m.selected = rowCursor(0) // a0, inside group a

	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(idA) {
		t.Fatalf("left did not fold row 0's own group (a)")
	}
	if m.isGroupCollapsed(idB) {
		t.Fatalf("left folded group b too, folding from a's own row")
	}
	if want := headerCursor(idA); m.selected != want {
		t.Fatalf("left folding from a row landed the cursor at %+v, want %+v (that group's own header)", m.selected, want)
	}
	if !m.isStopVisible(m.selected) {
		t.Fatalf("selection %+v landed on a hidden stop after folding its group", m.selected)
	}

	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("right did not unfold group a")
	}
	if want := headerCursor(idA); m.selected != want {
		t.Fatalf("right unfolding left the cursor at %+v, want %+v (unchanged from where left parked it)", m.selected, want)
	}
}

// TestLeftRightFoldUnfoldDefaultGroup proves the structural default group
// (sessionGroupID's sentinel 0 -- SPEC §11: "default is not a row... it
// always exists") folds and unfolds exactly like any real group, from
// both its own header and from one of its member rows.
func TestLeftRightFoldUnfoldDefaultGroup(t *testing.T) {
	idA := int64(1)
	m := groupTestModel([]store.Session{
		{ID: "a0", Name: "a0", CWD: "/work/a", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "d0", Name: "d0", CWD: "/work/d", Status: "idle"}, // no group: the implicit default
	})

	// From the default group's own header.
	m.selected = headerCursor(0)
	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(0) {
		t.Fatalf("left did not fold the default group from its own header")
	}
	if want := headerCursor(0); m.selected != want {
		t.Fatalf("left folding from the default header moved the cursor to %+v, want %+v", m.selected, want)
	}
	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(0) {
		t.Fatalf("right did not unfold the default group")
	}

	// From a row that belongs to the default group (d0, index 1).
	m.selected = rowCursor(1)
	updated, _ = m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(0) {
		t.Fatalf("left did not fold the default group from one of its own rows")
	}
	if want := headerCursor(0); m.selected != want {
		t.Fatalf("left folding from a default-group row landed the cursor at %+v, want %+v (default's own header)", m.selected, want)
	}
	if m.isGroupCollapsed(idA) {
		t.Fatalf("folding the default group folded group a too")
	}
	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(0) {
		t.Fatalf("right did not unfold the default group")
	}
}

// TestLeftRightFoldUnfoldEmptyDefinedGroup proves a group the user has
// defined but never put a session into (cure-01-02, SPEC §11: "A group
// the user defined but has not filled yet still renders") is foldable and
// unfoldable from its own header exactly like a populated one, with no
// panic and no effect on any other group -- there is nothing else the
// fold could hide, since the group has no rows.
func TestLeftRightFoldUnfoldEmptyDefinedGroup(t *testing.T) {
	idA := int64(1)
	emptyID := int64(99)
	m := groupTestModel([]store.Session{
		{ID: "a0", Name: "a0", CWD: "/work/a", Status: "idle", GroupName: "a", GroupID: &idA},
	})
	m.allGroups = []store.Group{{ID: emptyID, Name: "empty"}}

	m.selected = headerCursor(emptyID)
	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if !m.isGroupCollapsed(emptyID) {
		t.Fatalf("left did not fold the defined-but-empty group")
	}
	if m.isGroupCollapsed(idA) {
		t.Fatalf("folding the empty group folded group a too")
	}
	if want := headerCursor(emptyID); m.selected != want {
		t.Fatalf("left folding the empty group's own header moved the cursor to %+v, want %+v", m.selected, want)
	}

	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(emptyID) {
		t.Fatalf("right did not unfold the defined-but-empty group")
	}
}

// TestEveryGroupIndividuallyUnfoldableWithAllGroupsFolded proves that once
// every group in the sidebar is folded (a, b, c, and the always-present
// default), each one is STILL individually reachable and unfoldable on
// its own header -- unfolding one must never touch any of the others'
// collapse state, and every header stays a visible, selectable stop
// throughout (isStopVisible is unconditional for headers).
func TestEveryGroupIndividuallyUnfoldableWithAllGroupsFolded(t *testing.T) {
	m, idA, idB, idC := foldUnfoldTestModel()
	ids := []int64{idA, idB, idC, 0} // 0: the always-present default header

	for _, id := range ids {
		m.setGroupCollapsed(id, true)
	}
	for _, id := range ids {
		if !m.isGroupCollapsed(id) {
			t.Fatalf("setup: group %d did not fold", id)
		}
	}

	for _, id := range ids {
		m.selected = headerCursor(id)
		updated, _ := m.Update(key("right"))
		m = updated.(Model)
		if m.isGroupCollapsed(id) {
			t.Fatalf("right on group %d's header did not unfold it while every group was folded", id)
		}
		for _, other := range ids {
			if other == id {
				continue
			}
			if !m.isGroupCollapsed(other) {
				t.Fatalf("unfolding group %d also unfolded group %d", id, other)
			}
		}
		if !m.isStopVisible(headerCursor(id)) {
			t.Fatalf("group %d's header was not a visible stop", id)
		}
		// Re-fold it before moving to the next id, so every iteration
		// starts from the same "every group folded" premise.
		m.setGroupCollapsed(id, true)
	}
}

// TestRepeatedCFoldsNoMoreThanOneGroup is task 014/D.3's fix for
// setGroupCollapsed's own eviction defect: repeated `c` presses on a
// selection that never itself moves between presses must fold no more
// than the ONE group the cursor started on. Before this task's fix
// (unchanged all the way back to the launch sha 2752c9e), setGroupCollapsed
// evicted the selection to nearestVisibleSelection on every collapse,
// which could walk it onto a session belonging to a DIFFERENT group -- so
// a second `c` press (still just "press c again", nothing else) collapsed
// that other group too.
//
// Failure against 2752c9e (probe adapted to that sha's bare-int
// selection, since sidebarCursor/headerCursor did not exist yet -- see
// this task's commit message for the exact probe): after 2 repeated `c`
// presses starting on group "a"'s only row, `probe_repeated_c_test.go`
// reported "after 2 repeated c presses, 2 groups are collapsed (idA=true
// idB=true) -- want at most 1", because the first collapse's eviction
// landed the bare-int selection on group "b"'s row, and the second press
// collapsed that too.
func TestRepeatedCFoldsNoMoreThanOneGroup(t *testing.T) {
	m, idA, idB, idC := foldUnfoldTestModel()
	m.selected = rowCursor(0) // a0, inside group a

	for i := 0; i < 4; i++ {
		updated, _ := m.Update(key("c"))
		m = updated.(Model)
		collapsed := 0
		for _, id := range []int64{idA, idB, idC} {
			if m.isGroupCollapsed(id) {
				collapsed++
			}
		}
		if collapsed > 1 {
			t.Fatalf("after %d repeated c presses, %d groups are collapsed (a=%v b=%v c=%v) -- want at most 1", i+1, collapsed, m.isGroupCollapsed(idA), m.isGroupCollapsed(idB), m.isGroupCollapsed(idC))
		}
		if !m.isStopVisible(m.selected) {
			t.Fatalf("after %d repeated c presses, selection %+v is on a hidden stop", i+1, m.selected)
		}
	}
}

// TestLeftRightNoopUnderOverlaysAndWithNoSessions proves left/right behave
// like every other bare-letter/navigation binding: a no-op while help or
// the `i` detail dialog covers the sidebar, and a no-op (not a panic)
// with no session to resolve a group from.
func TestLeftRightNoopUnderOverlaysAndWithNoSessions(t *testing.T) {
	m, idA, _, _ := foldUnfoldTestModel()
	m.selected = rowCursor(0)

	m.help = true
	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("left folded a group while help was open")
	}
	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("right affected a group while help was open")
	}
	m.help = false

	m.detail = true
	updated, _ = m.Update(key("left"))
	m = updated.(Model)
	if m.isGroupCollapsed(idA) {
		t.Fatalf("left folded a group while the detail dialog was open")
	}
	m.detail = false

	empty := groupTestModel(nil)
	updated, _ = empty.Update(key("left"))
	_ = updated.(Model) // must not panic with no sessions to select from
	updated, _ = empty.Update(key("right"))
	_ = updated.(Model) // must not panic with no sessions to select from
}
