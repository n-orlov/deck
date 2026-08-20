package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestSortSessionsByAttentionOrdersGroupsExactly proves requirement 28's
// group order: waiting -> error -> running -> starting -> idle -> stopped,
// regardless of the input order (deliberately scrambled here) or of the
// sessions' names/IDs sorting some other way.
func TestSortSessionsByAttentionOrdersGroupsExactly(t *testing.T) {
	sessions := []store.Session{
		{ID: "s", Name: "stopped-one", Status: "stopped", StatusAt: 10},
		{ID: "i", Name: "idle-one", Status: "idle", StatusAt: 10},
		{ID: "r", Name: "running-one", Status: "running", StatusAt: 10},
		{ID: "w", Name: "waiting-one", Status: "waiting", StatusAt: 10},
		{ID: "e", Name: "error-one", Status: "error", StatusAt: 10},
		{ID: "t", Name: "starting-one", Status: "starting", StatusAt: 10},
	}
	got := sortSessionsByAttention(sessions)
	want := []string{"w", "e", "r", "t", "i", "s"}
	if len(got) != len(want) {
		t.Fatalf("got %d sessions, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got ID %q, want %q (full order: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestSortSessionsByAttentionWaitingOldestFirst proves the "oldest first"
// half of requirement 28 specifically for the waiting group: a later
// StatusAt (more recently became waiting) sorts after an earlier one.
func TestSortSessionsByAttentionWaitingOldestFirst(t *testing.T) {
	sessions := []store.Session{
		{ID: "newest", Status: "waiting", StatusAt: 300},
		{ID: "oldest", Status: "waiting", StatusAt: 100},
		{ID: "middle", Status: "waiting", StatusAt: 200},
	}
	got := sortSessionsByAttention(sessions)
	want := []string{"oldest", "middle", "newest"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got ID %q, want %q (full order: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestSortSessionsByAttentionTiesBrokenByID is requirement 29's own case: two
// sessions sharing both a status and a StatusAt (the "frozen clock" scenario)
// must still resolve to exactly one order, every time, via the documented ID
// tie-break — not via map iteration order or any other non-deterministic
// source.
func TestSortSessionsByAttentionTiesBrokenByID(t *testing.T) {
	sessions := []store.Session{
		{ID: "zebra", Status: "waiting", StatusAt: 500},
		{ID: "alpha", Status: "waiting", StatusAt: 500},
		{ID: "mike", Status: "waiting", StatusAt: 500},
	}
	want := []string{"alpha", "mike", "zebra"}

	for attempt := 0; attempt < 20; attempt++ {
		got := sortSessionsByAttention(sessions)
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("attempt %d, position %d: got ID %q, want %q (full order: %v)", attempt, i, got[i].ID, id, idsOf(got))
			}
		}
	}
}

// TestSortSessionsByAttentionUnknownStatusSortsLast documents the fallback
// for a status outside the requirement-28 enumeration (there is no reachable
// path to one today, see docs/reports/phase2b1-findings.md): it degrades to
// least-urgent rather than vanishing or jumping ahead of "stopped".
func TestSortSessionsByAttentionUnknownStatusSortsLast(t *testing.T) {
	sessions := []store.Session{
		{ID: "mystery", Status: "archived", StatusAt: 1},
		{ID: "s", Status: "stopped", StatusAt: 1},
	}
	got := sortSessionsByAttention(sessions)
	if got[0].ID != "s" || got[1].ID != "mystery" {
		t.Fatalf("got order %v, want [s mystery]", idsOf(got))
	}
}

// TestSortSessionsByAttentionDoesNotMutateInput proves the function returns
// a new slice: a caller preserving a selection by comparing against the
// original order is safe.
func TestSortSessionsByAttentionDoesNotMutateInput(t *testing.T) {
	sessions := []store.Session{
		{ID: "s", Status: "stopped", StatusAt: 1},
		{ID: "w", Status: "waiting", StatusAt: 1},
	}
	wantOrder := []string{sessions[0].ID, sessions[1].ID}

	_ = sortSessionsByAttention(sessions)

	for i, id := range wantOrder {
		if sessions[i].ID != id {
			t.Fatalf("input slice reordered at %d: got %q, want %q", i, sessions[i].ID, id)
		}
	}
}

func idsOf(sessions []store.Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}
