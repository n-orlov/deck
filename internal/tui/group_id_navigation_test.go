package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestNavigationPrimitivesRespectGroupIDKeyedCollapse is task 013's (R129
// part 3) own proof that switching collapse state from a display-name key
// to a durable group-id key (sessionGroupID) did not change any
// navigation primitive's OWN behaviour: visualOrder, visibleSessionIndices,
// nearestVisibleSelection, next/prevVisibleSelection, pageSelection and the
// g/G keys (tui.go's own case "g"/"G") all still walk visual order, skip a
// collapsed group's hidden rows, and never land selection on a hidden row
// -- now driven by a collapse toggled by id (setGroupCollapsed(int64, ...))
// rather than by name.
//
// Fixture: two real groups by explicit id -- "alpha" (id 1, two members)
// and "bravo" (id 2, one member) -- plus one implicit default-group member
// (no GroupID, GroupName ""). R129's alphabetical-with-default-last order
// puts the visual order at [a1, a2, b1, d1] (indices 1,2,3,0 in m.sessions,
// which is deliberately NOT already index order -- see
// group_visual_order_test.go's own non-adjacency precedent). Collapsing
// "alpha" (id 1) by id must hide a1/a2 from every primitive below while
// leaving b1/d1 fully reachable.
func TestNavigationPrimitivesRespectGroupIDKeyedCollapse(t *testing.T) {
	alphaID, bravoID := int64(1), int64(2)
	m := groupTestModel([]store.Session{
		{ID: "d1", Name: "d1", Status: "idle"},                                                   // idx0: implicit default
		{ID: "a1", Name: "a1", Status: "idle", GroupName: "alpha", GroupID: groupIDPtr(alphaID)}, // idx1
		{ID: "a2", Name: "a2", Status: "idle", GroupName: "alpha", GroupID: groupIDPtr(alphaID)}, // idx2
		{ID: "b1", Name: "b1", Status: "idle", GroupName: "bravo", GroupID: groupIDPtr(bravoID)}, // idx3
	})

	wantVisual := []int{1, 2, 3, 0}
	if got := m.visualOrder(); !intsEqual(got, wantVisual) {
		t.Fatalf("fixture sanity: visualOrder() = %v, want %v", got, wantVisual)
	}

	m.setGroupCollapsed(alphaID, true)

	t.Run("visibleSessionIndices", func(t *testing.T) {
		if got, want := m.visibleSessionIndices(), []int{3, 0}; !intsEqual(got, want) {
			t.Fatalf("visibleSessionIndices() = %v, want %v (alpha's a1/a2 hidden by its id, not its name)", got, want)
		}
	})

	t.Run("nearestVisibleSelection", func(t *testing.T) {
		// From a1 (idx1), now hidden: forward search lands on the next
		// visible row, b1 (idx3), never on the still-hidden a2 (idx2).
		if got := m.nearestVisibleSelection(1); got != 3 {
			t.Fatalf("nearestVisibleSelection(1) = %d, want 3 (b1, the nearest visible row forward)", got)
		}
	})

	t.Run("nextPrevVisibleSelection", func(t *testing.T) {
		// One nextVisibleSelection call from b1 (idx3) moves exactly one
		// visual row, to d1 (idx0) -- never landing on a1/a2.
		next, ok := m.nextVisibleSelection(3)
		if !ok || next != 0 {
			t.Fatalf("nextVisibleSelection(3) = (%d, %v), want (0, true)", next, ok)
		}
		// And prevVisibleSelection from b1 (idx3) must report none: the
		// only visual row before it, a2 (idx2), is hidden by the same
		// collapsed id.
		if _, ok := m.prevVisibleSelection(3); ok {
			t.Fatalf("prevVisibleSelection(3) reported a visible predecessor, want none (a1/a2 are both hidden)")
		}
	})

	t.Run("pageSelection", func(t *testing.T) {
		m.selected = 3 // b1, the first visible row
		if got, want := m.pageSelection(1), 0; got != want {
			t.Fatalf("pageSelection(1) from b1 = %d, want %d (one visual row down: d1, never landing on hidden alpha rows)", got, want)
		}
		if got, want := m.pageSelection(-5), 3; got != want {
			t.Fatalf("pageSelection(-5) overshooting past the top clamps at %d (b1, the first VISIBLE row), got %d", want, got)
		}
	})

	t.Run("gG", func(t *testing.T) {
		m.selected = 0
		updated, _ := m.Update(key("g"))
		got := updated.(Model)
		if got.selected != 3 {
			t.Fatalf("g landed on %d, want 3 (b1, the first visible row -- alpha's rows are hidden by id)", got.selected)
		}
		updated, _ = got.Update(key("G"))
		got = updated.(Model)
		if got.selected != 0 {
			t.Fatalf("G landed on %d, want 0 (d1, the last visible row)", got.selected)
		}
	})

	// Expanding again by the SAME id restores every hidden row.
	m.setGroupCollapsed(alphaID, false)
	if !m.isSessionVisible(1) || !m.isSessionVisible(2) {
		t.Fatalf("expanding alpha (id %d) again did not restore its rows' visibility", alphaID)
	}
}

// TestCollapsedGroupsSurviveModelRebuildFromUIState proves SPEC §11's
// "collapse state persists in ui_state" (task 013/R129 part 3) end to end:
// a group collapsed by id and persisted via persistCollapsedGroups (the
// `c` key's own command, tui.go) is still collapsed in a brand-new Model
// built over the SAME state.db -- exactly the "restarted client" shape
// layout_persistence_test.go already proves for layout_mode/sidebar_width.
// GetCollapsedGroups never validates an id against a live groups row (its
// own doc comment: "does not filter out an id that no longer resolves"),
// so this test persists an arbitrary id with no matching sessions/groups
// row at all -- the persistence layer itself is what is under test here,
// not groupSessions' bucketing.
func TestCollapsedGroupsSurviveModelRebuildFromUIState(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	settings := config.Settings{}
	model := New(db, settings, "")
	if model.isGroupCollapsed(42) {
		t.Fatalf("fresh state.db should degrade to nothing collapsed")
	}

	model.setGroupCollapsed(42, true)
	cmd := model.persistCollapsedGroups()
	if cmd == nil {
		t.Fatal("persistCollapsedGroups returned nil with a store attached")
	}
	msg := cmd()
	if persisted, ok := msg.(uiStatePersisted); !ok || persisted.err != nil {
		t.Fatalf("persistCollapsedGroups command = %+v, want a successful uiStatePersisted", msg)
	}

	persisted, err := db.GetCollapsedGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !persisted[42] {
		t.Fatalf("ui_state collapsed_groups = %v, want {42: true}", persisted)
	}

	// A restarted client is a fresh New(db, ...) call over the same
	// state.db; it must render the same group collapsed, not the default.
	restarted := New(db, settings, "")
	if !restarted.isGroupCollapsed(42) {
		t.Fatalf("restarted client lost collapsed group 42")
	}
	if restarted.isGroupCollapsed(99) {
		t.Fatalf("restarted client collapsed an id that was never persisted")
	}
}
