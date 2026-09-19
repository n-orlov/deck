package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestNavigationFollowsVisualOrderNotIndexOrder is the regression test for
// the operator-reported defect (002-steering.md, found by hand on
// 786dfde): groupSessions() paints rows bucketed by group, appending a
// later session into an EARLIER group when its group was already seen,
// while ↑/↓ (and, before this fix, PgUp/PgDn and `space`) stepped through
// m.sessions in INDEX order. The two orders only coincide when every
// group's sessions happen to be adjacent in m.sessions — which every
// other grouping test in this package arranges, making the bug invisible
// there. This fixture deliberately does NOT: it reproduces the operator's
// real four sessions (magpie, deck-dev, ralphd-dev, pytest-bdd-migration),
// where pytest-bdd-migration shares magpie's group but sits at the far
// end of m.sessions, so magpie's group is non-adjacent.
//
// Task 011 (R129) updated this fixture's expected visual order: groups
// now sort alphabetically, case-insensitively ("agent-sessions-tui" <
// "invp-ops-dev-agents" < "ralphd"), rather than by first appearance, so
// deck-dev's group (agent-sessions-tui) now leads even though magpie's
// group (invp-ops-dev-agents) appears first in m.sessions — the
// non-adjacency this fixture exists to exercise is unaffected either way.
func TestNavigationFollowsVisualOrderNotIndexOrder(t *testing.T) {
	sessions := []store.Session{
		{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents"},                               // idx0
		{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui"},                             // idx1
		{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd"},                                                 // idx2
		{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents"}, // idx3
	}
	m := groupTestModel(sessions)

	// Sanity-check the fixture actually reproduces the reported painted
	// order: row0 deck-dev(idx1, agent-sessions-tui), row1 magpie(idx0,
	// invp-ops-dev-agents), row2 pytest-bdd-migration(idx3, same group as
	// row1), row3 ralphd-dev(idx2, ralphd) — i.e. index order (0,1,2,3)
	// diverges from visual order (1,0,3,2).
	wantVisual := []int{1, 0, 3, 2}
	gotVisual := m.visualOrder()
	if len(gotVisual) != len(wantVisual) {
		t.Fatalf("visualOrder() = %v, want %v", gotVisual, wantVisual)
	}
	for i := range wantVisual {
		if gotVisual[i] != wantVisual[i] {
			t.Fatalf("visualOrder() = %v, want %v (fixture no longer reproduces the non-adjacent-workspace case)", gotVisual, wantVisual)
		}
	}

	// ↓ from the top visual row (idx1, deck-dev) must land on the NEXT
	// visual row (idx0, magpie), not skip it, and must never step
	// backwards in visual order.
	seen := []int{1}
	from := 1
	for i := 0; i < 3; i++ {
		next, ok := m.nextVisibleSelection(from)
		if !ok {
			t.Fatalf("nextVisibleSelection(%d) reported no next row after visiting %v", from, seen)
		}
		seen = append(seen, next)
		from = next
	}
	if got, want := seen, []int{1, 0, 3, 2}; !intsEqual(got, want) {
		t.Fatalf("successive ↓ from idx1 visited %v in that order, want %v (one visual row per press, in visual order, never backwards)", got, want)
	}
	// One more ↓ at the last visual row must not move (and must not
	// report ok, since there is nothing after it).
	if next, ok := m.nextVisibleSelection(from); ok || next != from {
		t.Fatalf("nextVisibleSelection(%d) at the last visual row = (%d, %v), want (%d, false)", from, next, ok, from)
	}

	// ↑ must retrace the same path in reverse.
	seenUp := []int{from}
	for i := 0; i < 3; i++ {
		prev, ok := m.prevVisibleSelection(from)
		if !ok {
			t.Fatalf("prevVisibleSelection(%d) reported no previous row after visiting %v", from, seenUp)
		}
		seenUp = append(seenUp, prev)
		from = prev
	}
	if got, want := seenUp, []int{2, 3, 0, 1}; !intsEqual(got, want) {
		t.Fatalf("successive ↑ from the last visual row visited %v, want %v", got, want)
	}
}

// TestPageSelectionFollowsVisualOrder proves PgUp/PgDn's page arithmetic
// (internal/tui/tui.go's pgup/pgdown cases, wired through pageSelection)
// also moves in visual rows, not raw m.sessions index arithmetic — the
// same defect the operator's report named at tui.go:587-597 alongside
// ↑/↓. Visual order under R129's alphabetical group order is
// [1, 0, 3, 2] (deck-dev, magpie, pytest-bdd-migration, ralphd-dev).
func TestPageSelectionFollowsVisualOrder(t *testing.T) {
	sessions := []store.Session{
		{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents"},
		{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui"},
		{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd"},
		{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents"},
	}
	m := groupTestModel(sessions)

	m.selected = 0                                       // magpie, visual row 1
	if got, want := m.pageSelection(1), 3; got != want { // one visual row down -> pytest-bdd-migration (idx3)
		t.Fatalf("pageSelection(1) from idx0 = %d, want %d", got, want)
	}
	m.selected = 3                                       // pytest-bdd-migration, visual row 2
	if got, want := m.pageSelection(1), 2; got != want { // one visual row down -> ralphd-dev (idx2)
		t.Fatalf("pageSelection(1) from idx3 = %d, want %d", got, want)
	}
	m.selected = 2                                       // ralphd-dev, last visual row
	if got, want := m.pageSelection(3), 2; got != want { // overshooting past the end clamps at the last row
		t.Fatalf("pageSelection(3) from the last visual row = %d, want %d (clamp, not wrap or overshoot)", got, want)
	}
	m.selected = 2
	if got, want := m.pageSelection(-3), 1; got != want { // overshooting past the start clamps at the top row -> deck-dev (idx1)
		t.Fatalf("pageSelection(-3) from the last visual row = %d, want %d (clamp at the top row)", got, want)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
