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
	fixture, agentSessionsTuiID, invpOpsDevAgentsID, ralphdID := navigationParityFixture()

	// Task 113's own trap, asserted rather than assumed: idx0 and idx3
	// must resolve to the SAME sessionGroupKey() key, or the rest of this
	// test would silently exercise the adjacent case every other grouping
	// test in this package already covers.
	if ws0, ws3 := sessionGroupKey(fixture[0]), sessionGroupKey(fixture[3]); ws0 != ws3 {
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
		headerCursor(agentSessionsTuiID),
		rowCursor(1),
		headerCursor(invpOpsDevAgentsID),
		rowCursor(0),
		rowCursor(3),
		headerCursor(ralphdID),
		rowCursor(2),
		headerCursor(0),
	}

	m := New(nil, config.Settings{}, "")
	m.sessions = fixture

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

// navigationParityFixture is the fixture both tests in this file measure
// against, factored out so the hidden-row guard below cannot silently
// drift onto an easier shape than the traversal test above uses: the
// operator-reported magpie/pytest-bdd-migration case, in which magpie
// (idx0) and pytest-bdd-migration (idx3) share the group
// "invp-ops-dev-agents" but are NOT adjacent in m.sessions (deck-dev and
// ralphd-dev, idx1/idx2, sit between them), and every row carries an
// EXPLICIT GroupID (never the CWD-basename fallback, and never two groups
// told apart by GroupName alone -- task 008's gotcha: a nil GroupID
// buckets under the shared sentinel 0 and collapses with the default
// group). A fresh slice per call, so neither test can observe the other's
// model.
func navigationParityFixture() (sessions []store.Session, agentSessionsTuiID, invpOpsDevAgentsID, ralphdID int64) {
	agentSessionsTuiID, invpOpsDevAgentsID, ralphdID = 1, 2, 3
	sessions = []store.Session{
		{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", GroupName: "invp-ops-dev-agents", GroupID: groupIDPtr(invpOpsDevAgentsID), Status: "waiting"},
		{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", GroupName: "agent-sessions-tui", GroupID: groupIDPtr(agentSessionsTuiID), Status: "waiting"},
		{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", GroupName: "ralphd", GroupID: groupIDPtr(ralphdID), Status: "waiting"},
		{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", GroupName: "invp-ops-dev-agents", GroupID: groupIDPtr(invpOpsDevAgentsID), Status: "waiting"},
	}
	return sessions, agentSessionsTuiID, invpOpsDevAgentsID, ralphdID
}

// TestNavigationNeverLandsOnAHiddenRow is this file's own half of task
// 014/D.3's parity requirement: with a group FOLDED, no navigation gesture
// and no fold gesture may ever leave m.selected on a row the sidebar is
// not painting (isStopVisible false). The traversal test above only walks
// an entirely expanded fixture, so it cannot see a regression in which a
// key steps into a collapsed group's hidden rows -- this test folds the
// non-adjacent two-member group ("invp-ops-dev-agents", idx0 and idx3) and
// re-runs the same gestures over the resulting visible list.
//
// It is deliberately written against m.Update (real key presses), not the
// navigation primitives directly, so it also covers the `c`/left/right
// fold path task 014 changed: setGroupCollapsed no longer evicts the
// cursor via nearestVisibleSelection, which means the "never on a hidden
// row" invariant now has to hold because of where folding PUTS the cursor
// (the folded group's own still-visible header) rather than because a
// blanket eviction swept it somewhere visible afterwards.
func TestNavigationNeverLandsOnAHiddenRow(t *testing.T) {
	fixture, agentSessionsTuiID, invpOpsDevAgentsID, ralphdID := navigationParityFixture()

	m := New(nil, config.Settings{}, "")
	m.sessions = fixture

	press := func(m Model, value string) Model {
		updated, _ := m.Update(key(value))
		return updated.(Model)
	}
	// mustBeVisible is the invariant itself, asserted after every single
	// gesture below rather than only at the end of each walk.
	mustBeVisible := func(t *testing.T, m Model, gesture string) {
		t.Helper()
		if !m.isStopVisible(m.selected) {
			t.Fatalf("%s left the cursor on a hidden stop %+v (isStopVisible false); visible stops are %v", gesture, m.selected, m.visibleSessionIndices())
		}
	}

	// Fold the two-member, non-adjacent group by pressing `c` from one of
	// its OWN rows (idx0, magpie): task 014 requires the cursor to land on
	// that same group's header -- which is visible -- never on a hidden
	// row and never on a neighbouring group's header.
	m.selected = rowCursor(0)
	m = press(m, "c")
	if !m.isGroupCollapsed(invpOpsDevAgentsID) {
		t.Fatalf("c from magpie (idx0) did not fold group %d", invpOpsDevAgentsID)
	}
	if want := headerCursor(invpOpsDevAgentsID); m.selected != want {
		t.Fatalf("c from a folding group's own row left the cursor at %+v, want %+v (that group's own header)", m.selected, want)
	}
	mustBeVisible(t, m, "c from magpie (idx0)")

	// The folded group's ROWS (idx0 and idx3) are gone from the visible
	// list; its own header, and every other group's header and rows, stay.
	wantVisible := []sidebarCursor{
		headerCursor(agentSessionsTuiID),
		rowCursor(1),
		headerCursor(invpOpsDevAgentsID),
		headerCursor(ralphdID),
		rowCursor(2),
		headerCursor(0),
	}
	if got := m.visibleSessionIndices(); !cursorsEqual(got, wantVisible) {
		t.Fatalf("with group %d folded, visibleSessionIndices() = %v, want %v", invpOpsDevAgentsID, got, wantVisible)
	}
	for _, hidden := range []sidebarCursor{rowCursor(0), rowCursor(3)} {
		if m.isStopVisible(hidden) {
			t.Fatalf("isStopVisible(%+v) = true with its group folded, want false", hidden)
		}
	}

	// ↓/↑ over the whole visible list: every landing visible, every
	// visible stop visited exactly once, the hidden rows never among them.
	for _, walk := range []struct {
		name string
		key  string
		want []sidebarCursor
	}{
		{name: "down", key: "j", want: wantVisible},
		{name: "up", key: "k", want: reversedCursors(wantVisible)},
	} {
		m.selected = walk.want[0]
		visited := []sidebarCursor{m.selected}
		for i := 1; i < len(walk.want); i++ {
			m = press(m, walk.key)
			mustBeVisible(t, m, walk.name+" press "+walk.key)
			visited = append(visited, m.selected)
		}
		if !cursorsEqual(visited, walk.want) {
			t.Fatalf("%s visited %v, want %v (the folded group's hidden rows must not be stops)", walk.name, visited, walk.want)
		}
		// One extra press at the end of the list must not move at all --
		// least of all into the folded group's hidden rows.
		last := m.selected
		m = press(m, walk.key)
		if m.selected != last {
			t.Fatalf("%s past the end of the visible list moved to %+v, want to stay at %+v", walk.name, m.selected, last)
		}
		mustBeVisible(t, m, walk.name+" past the end")
	}

	// g/G, pgup/pgdn and space all land on visible stops too; space, which
	// only ever answers with a ROW, must skip the folded group's waiting
	// rows (idx0/idx3) entirely even though they need attention.
	for _, gesture := range []string{"g", "G", "pgup", "pgdown", " ", " ", " ", " "} {
		m = press(m, gesture)
		mustBeVisible(t, m, "press "+gesture)
		if idx, ok := m.selected.SessionIndex(); ok && (idx == 0 || idx == 3) {
			t.Fatalf("press %q landed on row %d, a row hidden by the folded group %d", gesture, idx, invpOpsDevAgentsID)
		}
	}

	// Folding the remaining groups one by one, from wherever the cursor
	// currently sits, never strands it either: with everything folded the
	// visible list is headers only, and the cursor is on one of them.
	for _, gid := range []int64{agentSessionsTuiID, ralphdID, 0} {
		m.selected = headerCursor(gid)
		m = press(m, "left")
		mustBeVisible(t, m, "left on header of group "+string(rune('0'+gid)))
	}
	// A row cursor left pointing at an already-hidden row (a stale state a
	// reload can produce) is rescued by the fold gesture itself: `left` on
	// an already-folded group re-runs setGroupCollapsed, which puts the
	// cursor on that group's own visible header rather than leaving it on
	// the hidden row. `c` is deliberately NOT used here -- it would UNFOLD.
	m.selected = rowCursor(1)
	m = press(m, "left")
	if want := headerCursor(agentSessionsTuiID); m.selected != want {
		t.Fatalf("left from a hidden row left the cursor at %+v, want %+v (its own group's header)", m.selected, want)
	}
	mustBeVisible(t, m, "left from a row whose group is already folded")
	for _, c := range m.visibleSessionIndices() {
		if !c.IsHeader() {
			t.Fatalf("with every group folded, visibleSessionIndices() still contains the row %+v", c)
		}
	}
}

// reversedCursors is the walk-backwards expectation for the guard above.
func reversedCursors(in []sidebarCursor) []sidebarCursor {
	out := make([]sidebarCursor, len(in))
	for i, c := range in {
		out[len(in)-1-i] = c
	}
	return out
}
