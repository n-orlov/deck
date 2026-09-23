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
// puts the visual order at [H(alpha), a1, a2, H(bravo), b1, H(default),
// d1] (task 012/D.1: a header stop ahead of each bucket, rows at indices
// 1,2,3,0 in m.sessions, which is deliberately NOT already index order --
// see group_visual_order_test.go's own non-adjacency precedent). Collapsing
// "alpha" (id 1) by id must hide a1/a2 from every primitive below while
// leaving alpha's OWN header, b1, bravo's header, d1 and default's header
// fully reachable (a header is never hidden by its own group's collapse).
func TestNavigationPrimitivesRespectGroupIDKeyedCollapse(t *testing.T) {
	alphaID, bravoID := int64(1), int64(2)
	m := groupTestModel([]store.Session{
		{ID: "d1", Name: "d1", Status: "idle"},                                                   // idx0: implicit default
		{ID: "a1", Name: "a1", Status: "idle", GroupName: "alpha", GroupID: groupIDPtr(alphaID)}, // idx1
		{ID: "a2", Name: "a2", Status: "idle", GroupName: "alpha", GroupID: groupIDPtr(alphaID)}, // idx2
		{ID: "b1", Name: "b1", Status: "idle", GroupName: "bravo", GroupID: groupIDPtr(bravoID)}, // idx3
	})

	wantVisual := []sidebarCursor{
		headerCursor(alphaID), rowCursor(1), rowCursor(2),
		headerCursor(bravoID), rowCursor(3),
		headerCursor(0), rowCursor(0),
	}
	if got := m.visualOrder(); !cursorsEqual(got, wantVisual) {
		t.Fatalf("fixture sanity: visualOrder() = %v, want %v", got, wantVisual)
	}

	m.setGroupCollapsed(alphaID, true)

	t.Run("visibleSessionIndices", func(t *testing.T) {
		want := []sidebarCursor{headerCursor(alphaID), headerCursor(bravoID), rowCursor(3), headerCursor(0), rowCursor(0)}
		if got := m.visibleSessionIndices(); !cursorsEqual(got, want) {
			t.Fatalf("visibleSessionIndices() = %v, want %v (alpha's a1/a2 hidden by its id, not its name; alpha's own header stays visible)", got, want)
		}
	})

	t.Run("nearestVisibleSelection", func(t *testing.T) {
		// From a1 (idx1), now hidden: forward search lands on the next
		// visible STOP, bravo's own header, never on the still-hidden a2
		// (idx2) or straight past bravo's header onto b1.
		if got, want := m.nearestVisibleSelection(rowCursor(1)), headerCursor(bravoID); got != want {
			t.Fatalf("nearestVisibleSelection(rowCursor(1)) = %+v, want %+v (bravo's header, the nearest visible stop forward)", got, want)
		}
	})

	t.Run("nextPrevVisibleSelection", func(t *testing.T) {
		// One nextVisibleSelection call from b1 (idx3) moves exactly one
		// visual stop, to the default group's header -- never landing on
		// a1/a2, and never skipping straight to d1 past that header.
		next, ok := m.nextVisibleSelection(rowCursor(3))
		if !ok || next != headerCursor(0) {
			t.Fatalf("nextVisibleSelection(rowCursor(3)) = (%+v, %v), want (%+v, true)", next, ok, headerCursor(0))
		}
		// And prevVisibleSelection from b1 (idx3) lands on bravo's OWN
		// header -- the stop immediately ahead of it in visual order,
		// unaffected by alpha's a1/a2 being hidden.
		prev, ok := m.prevVisibleSelection(rowCursor(3))
		if !ok || prev != headerCursor(bravoID) {
			t.Fatalf("prevVisibleSelection(rowCursor(3)) = (%+v, %v), want (%+v, true) (bravo's own header)", prev, ok, headerCursor(bravoID))
		}
	})

	t.Run("pageSelection", func(t *testing.T) {
		m.selected = rowCursor(3) // b1
		if got, want := m.pageSelection(1), headerCursor(0); got != want {
			t.Fatalf("pageSelection(1) from b1 = %+v, want %+v (one visual stop down: the default group's header, never landing on hidden alpha rows)", got, want)
		}
		if got, want := m.pageSelection(-5), headerCursor(alphaID); got != want {
			t.Fatalf("pageSelection(-5) overshooting past the top clamps at %+v (alpha's header, the first VISIBLE stop), got %+v", want, got)
		}
	})

	t.Run("gG", func(t *testing.T) {
		m.selected = rowCursor(0)
		updated, _ := m.Update(key("g"))
		got := updated.(Model)
		if got.selected != headerCursor(alphaID) {
			t.Fatalf("g landed on %+v, want %+v (alpha's own header, the first visible stop -- alpha's ROWS are hidden by id, but not its header)", got.selected, headerCursor(alphaID))
		}
		updated, _ = got.Update(key("G"))
		got = updated.(Model)
		if got.selected != rowCursor(0) {
			t.Fatalf("G landed on %+v, want %+v (d1, the last visible stop)", got.selected, rowCursor(0))
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
