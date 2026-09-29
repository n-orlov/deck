package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestSortPinnedFirstAllOrders is R160's own table-driven success
// criterion: one group of pinned and unpinned rows of mixed status, and
// for every one of the four `sort_order` values every pinned row precedes
// every unpinned row, each tier keeps that order's own sequence (with its
// own id tie-break intact), and -- the attention order's own named case --
// a pinned `idle` row precedes an unpinned `waiting` row despite `waiting`
// outranking `idle` in the plain requirement-28 group order.
func TestSortPinnedFirstAllOrders(t *testing.T) {
	t.Run("attention", func(t *testing.T) {
		sessions := []store.Session{
			// Unpinned tier: two "waiting" rows tied on rank+StatusAt, no
			// previous frame, so the id tie-break alone decides their
			// relative order (un-a before un-b).
			{ID: "un-b", Status: "waiting", StatusAt: 200},
			{ID: "un-a", Status: "waiting", StatusAt: 200},
			// Pinned tier: an idle row, lower urgency than every unpinned
			// waiting row, must still land ABOVE all of them.
			{ID: "pin-idle", Status: "idle", StatusAt: 50, PinnedAt: 1},
			// Pinned tier: two more "waiting" rows tied on rank+StatusAt,
			// same id tie-break rule as the unpinned pair above.
			{ID: "pin-b", Status: "waiting", StatusAt: 300, PinnedAt: 1},
			{ID: "pin-a", Status: "waiting", StatusAt: 300, PinnedAt: 1},
		}
		got := sortSessionsByOrder(nil, sessions, SortOrderAttention)
		want := []string{"pin-a", "pin-b", "pin-idle", "un-a", "un-b"}
		assertIDOrder(t, got, want)
	})

	t.Run("created", func(t *testing.T) {
		sessions := []store.Session{
			{ID: "un-b", CreatedAt: 500},
			{ID: "un-a", CreatedAt: 500},
			{ID: "pin-low", CreatedAt: 10, PinnedAt: 1}, // pinned, oldest -- must still lead
			{ID: "pin-b", CreatedAt: 600, PinnedAt: 1},
			{ID: "pin-a", CreatedAt: 600, PinnedAt: 1},
		}
		got := sortSessionsByOrder(nil, sessions, SortOrderCreated)
		want := []string{"pin-a", "pin-b", "pin-low", "un-a", "un-b"}
		assertIDOrder(t, got, want)
	})

	t.Run("activity", func(t *testing.T) {
		sessions := []store.Session{
			{ID: "un-b", StatusAt: 500},
			{ID: "un-a", StatusAt: 500},
			{ID: "pin-low", StatusAt: 10, PinnedAt: 1}, // pinned, least recent -- must still lead
			{ID: "pin-b", StatusAt: 600, PinnedAt: 1},
			{ID: "pin-a", StatusAt: 600, PinnedAt: 1},
		}
		got := sortSessionsByOrder(nil, sessions, SortOrderActivity)
		want := []string{"pin-a", "pin-b", "pin-low", "un-a", "un-b"}
		assertIDOrder(t, got, want)
	})

	t.Run("name", func(t *testing.T) {
		sessions := []store.Session{
			{ID: "un-b", Name: "zebra"},
			{ID: "un-a", Name: "zebra"},
			{ID: "pin-low", Name: "zzzz", PinnedAt: 1}, // pinned, sorts last alphabetically -- must still lead
			{ID: "pin-b", Name: "apple", PinnedAt: 1},
			{ID: "pin-a", Name: "apple", PinnedAt: 1},
		}
		got := sortSessionsByOrder(nil, sessions, SortOrderName)
		want := []string{"pin-a", "pin-b", "pin-low", "un-a", "un-b"}
		assertIDOrder(t, got, want)
	})
}

// assertIDOrder is this file's own idsOf-based comparison, factored out
// since every sub-test above checks the same full-sequence shape.
func assertIDOrder(t *testing.T, got []store.Session, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (got %v)", len(got), len(want), idsOf(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestSortFreshPinMovesDespitePreviousFrame is the PRD's own named gotcha
// (phase4f-sidebar-pins.md: "the pinned key must sit above that
// [previous-frame] preference, or a freshly pinned row will stay where it
// was until an unrelated reorder"): a row with an established
// previous-frame position -- the middle of a three-way tie the
// previous-position tie-break would otherwise keep it in -- moves to the
// TOP of its group on the very next sort, immediately after being pinned,
// under all four orders.
func TestSortFreshPinMovesDespitePreviousFrame(t *testing.T) {
	cases := []struct {
		order string
		make  func(id string) store.Session
	}{
		{SortOrderAttention, func(id string) store.Session { return store.Session{ID: id, Status: "idle", StatusAt: 500} }},
		{SortOrderCreated, func(id string) store.Session { return store.Session{ID: id, CreatedAt: 500} }},
		{SortOrderActivity, func(id string) store.Session { return store.Session{ID: id, StatusAt: 500} }},
		{SortOrderName, func(id string) store.Session { return store.Session{ID: id, Name: "same-name"} }},
	}
	for _, tc := range cases {
		t.Run(tc.order, func(t *testing.T) {
			a, x, b := tc.make("a"), tc.make("x"), tc.make("b")
			// All three tie on the primary key, and previously appeared in
			// this exact relative order -- so absent any pin, the
			// previous-position tie-break would keep "x" in the middle on
			// this very next sort.
			previous := []store.Session{a, x, b}
			xPinned := x
			xPinned.PinnedAt = 42
			incoming := []store.Session{a, xPinned, b}
			got := sortSessionsByOrder(previous, incoming, tc.order)
			if got[0].ID != "x" {
				t.Fatalf("%s: freshly pinned row did not move to the top on the very next sort, got %v", tc.order, idsOf(got))
			}
		})
	}
}

// TestSortPinnedTierPerGroupKeepsGroupOrder proves the pinned tier is
// derived once over the WHOLE session list (sortSessionsByOrder, applied
// before grouping) and still yields a per-group pinned tier once
// groupSessions buckets the result, without ever moving a row to another
// group or touching group order (groupSortsBefore) itself.
func TestSortPinnedTierPerGroupKeepsGroupOrder(t *testing.T) {
	raw := []store.Session{
		{ID: "a1", GroupName: "teamA", Status: "waiting", StatusAt: 100},
		{ID: "a2", GroupName: "teamA", Status: "idle", StatusAt: 50, PinnedAt: 1},
		{ID: "b1", GroupName: "teamB", Status: "error", StatusAt: 80},
		{ID: "b2", GroupName: "teamB", Status: "stopped", StatusAt: 20, PinnedAt: 1},
	}
	sorted := sortSessionsByOrder(nil, raw, SortOrderAttention)
	m := groupTestModel(sorted)
	groups := m.groupSessions()

	// Group order itself is untouched: alphabetical, default last -- the
	// same rule as before this task, independent of any row's pin or
	// status.
	if len(groups) < 2 || groups[0].Name != "teamA" || groups[1].Name != "teamB" {
		names := make([]string, len(groups))
		for i, g := range groups {
			names[i] = g.Name
		}
		t.Fatalf("group order = %v, want [teamA, teamB, ...]", names)
	}

	teamA := groups[0].Sessions
	if len(teamA) != 2 || teamA[0].Session.ID != "a2" || teamA[1].Session.ID != "a1" {
		t.Fatalf("teamA bucket = %v, want [a2 (pinned), a1] -- pinned tier must sort first within its OWN group", idsOfIndexed(teamA))
	}

	teamB := groups[1].Sessions
	if len(teamB) != 2 || teamB[0].Session.ID != "b2" || teamB[1].Session.ID != "b1" {
		t.Fatalf("teamB bucket = %v, want [b2 (pinned), b1] -- pinned tier must sort first within its OWN group", idsOfIndexed(teamB))
	}
}

func idsOfIndexed(rows []indexedSession) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Session.ID
	}
	return ids
}

// TestAttentionWalkReachesWaitingBelowPinned is R160's own named test:
// `space` from a pinned row reaches an unpinned `waiting` row that sits
// below it (the sort places pinned rows first, so a genuinely urgent
// unpinned row can now sit anywhere below one), and the collapsed strip's
// attention count still counts it -- pinning never hides urgency from
// either surface, since both go through the same NeedsAttention answer
// that never looks at PinnedAt at all.
func TestAttentionWalkReachesWaitingBelowPinned(t *testing.T) {
	m := Model{sessions: []store.Session{
		{ID: "pin-idle", Status: "idle", PinnedAt: 1},
		{ID: "un-waiting", Status: "waiting"},
	}}
	if got, ok := m.nextAttentionSelection(rowCursor(0)); !ok || got != rowCursor(1) {
		t.Fatalf("space from the pinned row: got (%+v, %v), want (rowCursor(1), true)", got, ok)
	}
	if count := m.attentionCount(); count != 1 {
		t.Fatalf("attentionCount() = %d, want 1 (the unpinned waiting row below the pinned one)", count)
	}
}
