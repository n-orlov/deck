package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestNavigationVisitsEveryVisualRowInBothGroupingModes is task 009's (I-6,
// requirement 36) proof that ↑/↓, g/G and space visit rows in the visual
// order the sidebar actually renders -- in BOTH grouping modes (task 008's
// I-5 on/off switch) -- with no row unreachable and no row visited twice.
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
// In flat mode (grouping off) this non-adjacency is irrelevant -- visual
// order is simply index order -- which is itself the point: the same
// fixture must drive identical navigation guarantees under both switches
// of task 007/008's config knob.
func TestNavigationVisitsEveryVisualRowInBothGroupingModes(t *testing.T) {
	fixture := func() []store.Session {
		return []store.Session{
			{ID: "magpie", Name: "magpie", CWD: "/home/x/invp-ops-dev-agents", Workspace: "invp-ops-dev-agents", Status: "waiting"},
			{ID: "deck-dev", Name: "deck-dev", CWD: "/home/x/agent-sessions-tui", Workspace: "agent-sessions-tui", Status: "waiting"},
			{ID: "ralphd-dev", Name: "ralphd-dev", CWD: "/home/x/ralphd", Workspace: "ralphd", Status: "waiting"},
			{ID: "pytest-bdd-migration", Name: "pytest-bdd-migration", CWD: "/home/x/invp-ops-dev-agents-2", Workspace: "invp-ops-dev-agents", Status: "waiting"},
		}
	}

	// Task 113's own trap, asserted rather than assumed: idx0 and idx3
	// must resolve to the SAME sessionWorkspace() key, or the rest of this
	// test would silently exercise the adjacent case every other grouping
	// test in this package already covers.
	if ws0, ws3 := sessionWorkspace(fixture()[0]), sessionWorkspace(fixture()[3]); ws0 != ws3 {
		t.Fatalf("fixture sanity: sessionWorkspace(idx0)=%q != sessionWorkspace(idx3)=%q, want equal (task 113 trap)", ws0, ws3)
	}

	cases := []struct {
		name       string
		grouping   bool
		wantVisual []int
	}{
		// Grouped: idx0 (magpie) and idx3 (pytest-bdd-migration) bucket
		// together since they share a workspace, giving painted order
		// 0,3,1,2 -- matches TestNavigationFollowsVisualOrderNotIndexOrder.
		{"grouped", true, []int{0, 3, 1, 2}},
		// Flat (requirement 35): no buckets, so painted order is simply
		// index order -- the non-adjacency that matters in grouped mode is
		// a non-event here.
		{"flat", false, []int{0, 1, 2, 3}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{GroupByWorkspace: tc.grouping}, "")
			m.sessions = fixture()

			// Assert the fixture reproduces the intended painted order for
			// THIS mode before trusting anything measured against it.
			if got := m.visualOrder(); !intsEqual(got, tc.wantVisual) {
				t.Fatalf("visualOrder() (%s) = %v, want %v (fixture no longer reproduces the non-adjacent-same-workspace case)", tc.name, got, tc.wantVisual)
			}

			press := func(m Model, key tea.KeyMsg) Model {
				updated, _ := m.Update(key)
				return updated.(Model)
			}
			runeKey := func(s string) tea.KeyMsg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
			}

			n := len(tc.wantVisual)

			// ↓ ("j"): from the first visual row, n-1 successive presses
			// must visit every remaining visual row EXACTLY ONCE, forward,
			// never skipping and never revisiting.
			m.selected = tc.wantVisual[0]
			visitedDown := []int{m.selected}
			for i := 1; i < n; i++ {
				m = press(m, runeKey("j"))
				visitedDown = append(visitedDown, m.selected)
			}
			if !intsEqual(visitedDown, tc.wantVisual) {
				t.Fatalf("↓ (%s) visited %v, want %v", tc.name, visitedDown, tc.wantVisual)
			}
			// One more ↓ at the last visual row must not move (nothing
			// unreachable, nothing beyond the end either).
			last := m.selected
			m = press(m, runeKey("j"))
			if m.selected != last {
				t.Fatalf("↓ (%s) at the last visual row moved to %d, want to stay at %d", tc.name, m.selected, last)
			}

			// ↑ ("k") must retrace the exact same path in reverse.
			wantUp := make([]int, n)
			for i, idx := range tc.wantVisual {
				wantUp[n-1-i] = idx
			}
			visitedUp := []int{m.selected}
			for i := 1; i < n; i++ {
				m = press(m, runeKey("k"))
				visitedUp = append(visitedUp, m.selected)
			}
			if !intsEqual(visitedUp, wantUp) {
				t.Fatalf("↑ (%s) visited %v, want %v", tc.name, visitedUp, wantUp)
			}
			first := m.selected
			m = press(m, runeKey("k"))
			if m.selected != first {
				t.Fatalf("↑ (%s) at the first visual row moved to %d, want to stay at %d", tc.name, m.selected, first)
			}

			// g/G: jump to the first/last visual row from anywhere.
			m.selected = tc.wantVisual[n/2]
			m = press(m, runeKey("g"))
			if want := tc.wantVisual[0]; m.selected != want {
				t.Fatalf("g (%s) landed on %d, want %d (first visual row)", tc.name, m.selected, want)
			}
			m.selected = tc.wantVisual[n/2]
			m = press(m, runeKey("G"))
			if want := tc.wantVisual[n-1]; m.selected != want {
				t.Fatalf("G (%s) landed on %d, want %d (last visual row)", tc.name, m.selected, want)
			}

			// space: every session in the fixture is "waiting"
			// (NeedsAttention true for all four), so successive space
			// presses from the first visual row must step through every
			// OTHER visual row exactly once, in visual order, then wrap
			// back to the first -- never skipping and never revisiting,
			// exactly like ↓ but with wraparound (nextAttentionSelection,
			// internal/tui/tui.go, walks m.visualOrder()).
			m.selected = tc.wantVisual[0]
			visitedSpace := []int{m.selected}
			for i := 1; i < n; i++ {
				m = press(m, tea.KeyMsg{Type: tea.KeySpace})
				visitedSpace = append(visitedSpace, m.selected)
			}
			if !intsEqual(visitedSpace, tc.wantVisual) {
				t.Fatalf("space (%s) visited %v, want %v", tc.name, visitedSpace, tc.wantVisual)
			}
			m = press(m, tea.KeyMsg{Type: tea.KeySpace})
			if want := tc.wantVisual[0]; m.selected != want {
				t.Fatalf("space (%s) after visiting every row wrapped to %d, want %d (first visual row)", tc.name, m.selected, want)
			}
		})
	}
}
