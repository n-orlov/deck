package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestNavigationVisitsEveryVisualRow is task 009's (I-6, requirement 36)
// proof that ↑/↓, g/G and space visit rows in the visual order the sidebar
// actually renders, with no row unreachable and no row visited twice.
// R129 part 2 removed the flat/ungrouped mode this test used to parametrise
// over (config.Settings{}.GroupByWorkspace is gone; grouping is now
// unconditional) -- the grouped half survives as its own test, per the
// standing rule that a test proving flat mode is deleted with the
// behaviour rather than re-aimed.
//
// The fixture is group_visual_order_test.go's own operator-reported
// magpie/pytest-bdd-migration case: magpie (idx0) and pytest-bdd-migration
// (idx3) share a workspace but are NOT adjacent in m.sessions (deck-dev and
// ralphd-dev, idx1/idx2, sit between them) -- exactly the shape
// docs/reports/phase3-findings.md's task 113 finding warns a fixture must
// get right (two same-workspace rows that are non-adjacent in the
// already-sorted array), and both rows carry an EXPLICIT Workspace field
// (never relying on the CWD-basename fallback), which is task 113's own
// fix for the "different scratch-dir basenames land in two groups" trap.
//
// Task 012/D.1 made every group's header its own visual STOP: ↑/↓ and g/G
// now step onto one exactly like a row, so "every visual row" below reads
// "every visual STOP" -- wantVisualRows strips the three header stops back
// out wherever a check (the fixture sanity check, and space's
// attention-only walk, which never lands on a header) still means "rows
// only".
func TestNavigationVisitsEveryVisualRow(t *testing.T) {
	agentSessionsTuiID := groupIDPtr(1)
	invpOpsDevAgentsID := groupIDPtr(2)
	ralphdID := groupIDPtr(3)
	fixture := func() []store.Session {
		return []store.Session{
			{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID, Status: "waiting"},
			{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui", GroupID: agentSessionsTuiID, Status: "waiting"},
			{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd", GroupID: ralphdID, Status: "waiting"},
			{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents", GroupID: invpOpsDevAgentsID, Status: "waiting"},
		}
	}

	// Task 113's own trap, asserted rather than assumed: idx0 and idx3
	// must resolve to the SAME sessionGroupKey() key, or the rest of this
	// test would silently exercise the adjacent case every other grouping
	// test in this package already covers.
	if ws0, ws3 := sessionGroupKey(fixture()[0]), sessionGroupKey(fixture()[3]); ws0 != ws3 {
		t.Fatalf("fixture sanity: sessionGroupKey(idx0)=%q != sessionGroupKey(idx3)=%q, want equal (task 113 trap)", ws0, ws3)
	}

	// Alphabetical, case-insensitive group order (R129, task 011) puts
	// "agent-sessions-tui" (deck-dev, idx1) first, then
	// "invp-ops-dev-agents" (magpie idx0 and pytest-bdd-migration idx3,
	// which bucket together since they share a group), then "ralphd"
	// (ralphd-dev, idx2) -- painted row order 1,0,3,2, with a header stop
	// ahead of each of the three buckets.
	wantVisualRows := []int{1, 0, 3, 2}
	wantVisual := []sidebarCursor{
		headerCursor(*agentSessionsTuiID),
		rowCursor(1),
		headerCursor(*invpOpsDevAgentsID),
		rowCursor(0),
		rowCursor(3),
		headerCursor(*ralphdID),
		rowCursor(2),
		headerCursor(0),
	}

	m := New(nil, config.Settings{}, "")
	m.sessions = fixture()

	// Assert the fixture reproduces the intended painted order before
	// trusting anything measured against it.
	if got := m.visualOrder(); !cursorsEqual(got, wantVisual) {
		t.Fatalf("visualOrder() = %v, want %v (fixture no longer reproduces the non-adjacent-same-workspace case)", got, wantVisual)
	}

	press := func(m Model, key tea.KeyMsg) Model {
		updated, _ := m.Update(key)
		return updated.(Model)
	}
	runeKey := func(s string) tea.KeyMsg {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}

	n := len(wantVisual)

	// ↓ ("j"): from the first visual stop, n-1 successive presses must
	// visit every remaining visual stop EXACTLY ONCE, forward, never
	// skipping and never revisiting.
	m.selected = wantVisual[0]
	visitedDown := []sidebarCursor{m.selected}
	for i := 1; i < n; i++ {
		m = press(m, runeKey("j"))
		visitedDown = append(visitedDown, m.selected)
	}
	if !cursorsEqual(visitedDown, wantVisual) {
		t.Fatalf("↓ visited %v, want %v", visitedDown, wantVisual)
	}
	// One more ↓ at the last visual stop must not move (nothing
	// unreachable, nothing beyond the end either).
	last := m.selected
	m = press(m, runeKey("j"))
	if m.selected != last {
		t.Fatalf("↓ at the last visual stop moved to %+v, want to stay at %+v", m.selected, last)
	}

	// ↑ ("k") must retrace the exact same path in reverse.
	wantUp := make([]sidebarCursor, n)
	for i, c := range wantVisual {
		wantUp[n-1-i] = c
	}
	visitedUp := []sidebarCursor{m.selected}
	for i := 1; i < n; i++ {
		m = press(m, runeKey("k"))
		visitedUp = append(visitedUp, m.selected)
	}
	if !cursorsEqual(visitedUp, wantUp) {
		t.Fatalf("↑ visited %v, want %v", visitedUp, wantUp)
	}
	first := m.selected
	m = press(m, runeKey("k"))
	if m.selected != first {
		t.Fatalf("↑ at the first visual stop moved to %+v, want to stay at %+v", m.selected, first)
	}

	// g/G: jump to the first/last visual stop from anywhere.
	m.selected = wantVisual[n/2]
	m = press(m, runeKey("g"))
	if want := wantVisual[0]; m.selected != want {
		t.Fatalf("g landed on %+v, want %+v (first visual stop)", m.selected, want)
	}
	m.selected = wantVisual[n/2]
	m = press(m, runeKey("G"))
	if want := wantVisual[n-1]; m.selected != want {
		t.Fatalf("G landed on %+v, want %+v (last visual stop)", m.selected, want)
	}

	// space: every session in the fixture is "waiting" (NeedsAttention
	// true for all four), so successive space presses from the first
	// visual ROW must step through every OTHER visual row exactly once,
	// in visual order, then wrap back to the first -- never skipping and
	// never revisiting, exactly like ↓ but with wraparound and never
	// landing on a header (nextAttentionSelection, internal/tui/tui.go,
	// walks m.visualOrder() but only ever answers with a row cursor).
	m.selected = rowCursor(wantVisualRows[0])
	visitedSpace := []int{wantVisualRows[0]}
	for i := 1; i < len(wantVisualRows); i++ {
		m = press(m, tea.KeyMsg{Type: tea.KeySpace})
		idx, ok := m.selected.SessionIndex()
		if !ok {
			t.Fatalf("space landed on a header cursor %+v, want a row", m.selected)
		}
		visitedSpace = append(visitedSpace, idx)
	}
	if !intsEqual(visitedSpace, wantVisualRows) {
		t.Fatalf("space visited %v, want %v", visitedSpace, wantVisualRows)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})
	gotIdx, ok := m.selected.SessionIndex()
	if !ok {
		t.Fatalf("space after visiting every row landed on a header cursor %+v, want a row", m.selected)
	}
	if want := wantVisualRows[0]; gotIdx != want {
		t.Fatalf("space after visiting every row wrapped to row %d, want row %d (first visual row)", gotIdx, want)
	}
}
