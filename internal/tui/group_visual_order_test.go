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
//
// Task 012/D.1 gave every group's header its own visual STOP, so the
// expected sequence below is header, row, header, row, row, header, row
// -- one header per bucket, ahead of its member rows -- rather than the
// four bare row indices this test asserted before headers were
// navigable. Each group carries a distinct, real GroupID (groupIDPtr)
// so the three header cursors are themselves distinguishable stops, not
// three copies of the same sidebarCursor value.
func TestNavigationFollowsVisualOrderNotIndexOrder(t *testing.T) {
	agentSessionsTuiID := groupIDPtr(1)
	invpOpsDevAgentsID := groupIDPtr(2)
	ralphdID := groupIDPtr(3)
	sessions := []store.Session{
		{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID},                               // idx0
		{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui", GroupID: agentSessionsTuiID},                             // idx1
		{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd", GroupID: ralphdID},                                                           // idx2
		{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID}, // idx3
	}
	m := groupTestModel(sessions)

	// Sanity-check the fixture actually reproduces the reported painted
	// order: header(agent-sessions-tui), row1 (deck-dev), header
	// (invp-ops-dev-agents), row0 (magpie), row3 (pytest-bdd-migration,
	// same group as row0), header(ralphd), row2 (ralphd-dev), then the
	// implicit default group's own header last -- cure-01-02's own rule
	// (SPEC §11: "default is not a row... it always exists") seeds it
	// unconditionally while unfiltered, even with no default-group member
	// here at all -- i.e. index order (0,1,2,3) diverges from visual row
	// order (1,0,3,2), with a header stop ahead of each bucket.
	want := []sidebarCursor{
		headerCursor(*agentSessionsTuiID),
		rowCursor(1),
		headerCursor(*invpOpsDevAgentsID),
		rowCursor(0),
		rowCursor(3),
		headerCursor(*ralphdID),
		rowCursor(2),
		headerCursor(0),
	}
	got := m.visualOrder()
	if !cursorsEqual(got, want) {
		t.Fatalf("visualOrder() = %v, want %v (fixture no longer reproduces the non-adjacent-workspace case)", got, want)
	}

	// ↓ from the top visual stop (the agent-sessions-tui header) must
	// walk every stop above, in order, never skipping and never
	// stepping backwards -- a header is a stop now exactly like a row.
	from := want[0]
	seen := []sidebarCursor{from}
	for i := 0; i < len(want)-1; i++ {
		next, ok := m.nextVisibleSelection(from)
		if !ok {
			t.Fatalf("nextVisibleSelection(%+v) reported no next stop after visiting %v", from, seen)
		}
		seen = append(seen, next)
		from = next
	}
	if !cursorsEqual(seen, want) {
		t.Fatalf("successive ↓ from the first visual stop visited %v in that order, want %v (one visual stop per press, in visual order, never backwards)", seen, want)
	}
	// One more ↓ at the last visual stop must not move (and must not
	// report ok, since there is nothing after it).
	if next, ok := m.nextVisibleSelection(from); ok || next != from {
		t.Fatalf("nextVisibleSelection(%+v) at the last visual stop = (%+v, %v), want (%+v, false)", from, next, ok, from)
	}

	// ↑ must retrace the same path in reverse.
	wantUp := make([]sidebarCursor, len(want))
	for i, c := range want {
		wantUp[len(want)-1-i] = c
	}
	seenUp := []sidebarCursor{from}
	for i := 0; i < len(wantUp)-1; i++ {
		prev, ok := m.prevVisibleSelection(from)
		if !ok {
			t.Fatalf("prevVisibleSelection(%+v) reported no previous stop after visiting %v", from, seenUp)
		}
		seenUp = append(seenUp, prev)
		from = prev
	}
	if !cursorsEqual(seenUp, wantUp) {
		t.Fatalf("successive ↑ from the last visual stop visited %v, want %v", seenUp, wantUp)
	}
}

// TestPageSelectionFollowsVisualOrder proves PgUp/PgDn's page arithmetic
// (internal/tui/tui.go's pgup/pgdown cases, wired through pageSelection)
// also moves in visual STOPS -- header included, task 012/D.1 -- not raw
// m.sessions index arithmetic — the same defect the operator's report
// named at tui.go:587-597 alongside ↑/↓. Visual order under R129's
// alphabetical group order is header(agent-sessions-tui), row1
// (deck-dev), header(invp-ops-dev-agents), row0 (magpie), row3
// (pytest-bdd-migration), header(ralphd), row2 (ralphd-dev).
func TestPageSelectionFollowsVisualOrder(t *testing.T) {
	agentSessionsTuiID := groupIDPtr(1)
	invpOpsDevAgentsID := groupIDPtr(2)
	ralphdID := groupIDPtr(3)
	sessions := []store.Session{
		{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID},
		{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui", GroupID: agentSessionsTuiID},
		{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd", GroupID: ralphdID},
		{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID},
	}
	m := groupTestModel(sessions)

	m.selected = headerCursor(*agentSessionsTuiID) // the very first visual stop
	if got, want := m.pageSelection(1), rowCursor(1); got != want {
		t.Fatalf("pageSelection(1) from the first header = %+v, want %+v (deck-dev's row, the next stop)", got, want)
	}
	m.selected = rowCursor(0) // magpie
	if got, want := m.pageSelection(1), rowCursor(3); got != want {
		t.Fatalf("pageSelection(1) from idx0 = %+v, want %+v (pytest-bdd-migration, the next stop after magpie's own group header)", got, want)
	}
	m.selected = rowCursor(3) // pytest-bdd-migration
	if got, want := m.pageSelection(1), headerCursor(*ralphdID); got != want {
		t.Fatalf("pageSelection(1) from idx3 = %+v, want %+v (ralphd's header, the next stop)", got, want)
	}
	m.selected = rowCursor(2) // ralphd-dev, second-to-last visual stop
	if got, want := m.pageSelection(3), headerCursor(0); got != want {
		t.Fatalf("pageSelection(3) overshooting the end clamps at %+v (the implicit default group's own always-present header, the true last stop), got %+v", want, got)
	}
	m.selected = rowCursor(2)
	if got, want := m.pageSelection(-6), headerCursor(*agentSessionsTuiID); got != want {
		t.Fatalf("pageSelection(-6) from the last visual stop = %+v, want %+v (clamp at the top stop)", got, want)
	}
}

// cursorsEqual compares two []sidebarCursor sequences by VALUE, in
// order -- the []sidebarCursor counterpart of intsEqual below, used by
// every test in this file (and navigation_parity_test.go) that asserts a
// visual-order or traversal sequence now that a visual stop is a struct,
// not a bare int.
func cursorsEqual(a, b []sidebarCursor) bool {
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

// testSelectedSession resolves m.selected as a session -- store.Session{}
// (the zero value) when the cursor is a header, or a row's index has
// drifted out of bounds -- for every test in this package that indexes
// m.sessions[m.selected] the way it did back when m.selected was a bare
// int (task 012/D.1 made that a compile error: SessionIndex() is now the
// only way to read a session index out of a sidebarCursor).
func testSelectedSession(m Model) store.Session {
	idx, ok := m.selected.SessionIndex()
	if !ok || idx < 0 || idx >= len(m.sessions) {
		return store.Session{}
	}
	return m.sessions[idx]
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
