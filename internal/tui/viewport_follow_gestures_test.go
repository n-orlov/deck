package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 008/C.2 (#31, R136): one regression test per binding in
// R136's table (SPEC §11, PRD "Where the code is"), each proving that the
// selection viewport actually FOLLOWS the cursor for that specific gesture,
// plus the wheel's documented independence and the 80x24 flush floor.
// Every fixture is taller than the sidebar (so the list can genuinely
// scroll) and every test seeds m.sidebarScroll to a value that is valid
// for the PRE-gesture state but leaves the POST-gesture selection out of
// view unless the gesture's own call site follows it -- exactly the gap
// that existed at launch sha 2752c9e, before task 007 wired m.setSelection
// into any of these call sites. See the task 008 commit message for the
// exact failure text each assertion below produces when run against that
// sha.

// viewportFollowTestModel builds n sessions in one real group ("grp"), all
// distinguishable by ID/Name, at the given terminal height -- tall enough
// that sidebarScroll actually has room to differ from 0 once n is large
// enough relative to height.
func viewportFollowTestModel(n, height int) Model {
	var sessions []store.Session
	for i := 0; i < n; i++ {
		sessions = append(sessions, store.Session{
			ID:        fmt.Sprintf("s%02d", i),
			Name:      fmt.Sprintf("s%02d", i),
			CWD:       "/work/grp",
			GroupName: "grp",
			Status:    "idle",
		})
	}
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	return m
}

// viewportFollowFilterTestModel builds two real groups distinguished so a
// filter query can narrow the list down to exactly one of them: "keep"
// sessions first (indices 0..nKeep-1), "drop" sessions after
// (nKeep..nKeep+nDrop-1) -- selecting the LAST drop session and then
// filtering down to "keep" alone forces nearestVisibleSelection to clamp
// onto the last KEEP session (task 008 gotcha: the old index is simply out
// of range for the narrowed array), landing the cursor at the very bottom
// of the newly-filtered list.
func viewportFollowFilterTestModel(nKeep, nDrop, height int) Model {
	var sessions []store.Session
	for i := 0; i < nKeep; i++ {
		sessions = append(sessions, store.Session{ID: fmt.Sprintf("keep%02d", i), Name: fmt.Sprintf("keep%02d", i), CWD: "/work/keep", GroupName: "keep"})
	}
	for i := 0; i < nDrop; i++ {
		sessions = append(sessions, store.Session{ID: fmt.Sprintf("drop%02d", i), Name: fmt.Sprintf("drop%02d", i), CWD: "/work/drop", GroupName: "drop"})
	}
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	return m
}

// assertSelectionInView is the one invariant every gesture below is
// checked against: R136's own requirement, "the selected row's entry
// lines fall inside [sidebarScroll, sidebarScroll+contentHeight)".
func assertSelectionInView(t *testing.T, m Model, label string) {
	t.Helper()
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	start, end, _, _ := selectionSpan(entries, m.selected)
	if start == -1 {
		t.Fatalf("%s: selected session %d has no row entries in the current sidebar (hidden by a collapsed group?)", label, m.selected)
	}
	if m.sidebarScroll < 0 || start < m.sidebarScroll || end >= m.sidebarScroll+contentHeight {
		t.Fatalf("%s: sidebarScroll = %d leaves selection span [%d,%d] outside window [%d,%d)", label, m.sidebarScroll, start, end, m.sidebarScroll, m.sidebarScroll+contentHeight)
	}
}

// maxSidebarOffset is the flush-at-the-end bound clampSidebarScroll itself
// enforces, computed independently so a test can seed a scroll value that
// was valid a moment ago and assert the gesture corrects it.
func maxSidebarOffset(m Model) int {
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	return max(0, len(entries)-contentHeight)
}

// TestViewportFollowsUpAndK is R136's row 1: `up` and `k` both drive
// prevVisibleSelection, which (pre-task-007) assigned m.selected directly
// with no follow. Failure against 2752c9e: starting flush at the top
// (sidebarScroll=0) with the selection on the LAST session, one step
// backwards lands on session 28 while sidebarScroll stays 0 -- "up: sidebarScroll
// = 0 leaves selection span [57,58] outside window [0,10)" (and the same for
// "k").
func TestViewportFollowsUpAndK(t *testing.T) {
	for _, k := range []string{"up", "k"} {
		t.Run(k, func(t *testing.T) {
			m := viewportFollowTestModel(30, 13)
			m.selected = 29
			m.sidebarScroll = 0
			updated, _ := m.Update(key(k))
			m = updated.(Model)
			if m.selected != 28 {
				t.Fatalf("%s: selected = %d, want 28", k, m.selected)
			}
			assertSelectionInView(t, m, k)
		})
	}
}

// TestViewportFollowsDownAndJ is R136's row 1's other half: `down`/`j`
// drive nextVisibleSelection. Failure against 2752c9e: starting flush at
// the bottom (sidebarScroll = maxSidebarOffset, 52 for this fixture) with
// the selection on the FIRST session, one step forward lands on session 1
// while sidebarScroll stays 52 -- "down: sidebarScroll = 52 leaves selection
// span [3,4] outside window [52,62)" (and the same for "j").
func TestViewportFollowsDownAndJ(t *testing.T) {
	for _, k := range []string{"down", "j"} {
		t.Run(k, func(t *testing.T) {
			m := viewportFollowTestModel(30, 13)
			m.selected = 0
			m.sidebarScroll = maxSidebarOffset(m)
			updated, _ := m.Update(key(k))
			m = updated.(Model)
			if m.selected != 1 {
				t.Fatalf("%s: selected = %d, want 1", k, m.selected)
			}
			assertSelectionInView(t, m, k)
		})
	}
}

// TestViewportFollowsPgUp is R136's row 2, up half: pgup drives
// pageSelection(-sidebarRowsPerPage()). Failure against 2752c9e: starting
// flush at the top with the selection on the last session, one page up
// lands on session 24 while sidebarScroll stays 0 -- "pgup: sidebarScroll =
// 0 leaves selection span [49,50] outside window [0,10)".
func TestViewportFollowsPgUp(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.selected = 29
	m.sidebarScroll = 0
	updated, _ := m.Update(key("pgup"))
	m = updated.(Model)
	if m.selected == 29 {
		t.Fatalf("pgup did not move the selection (fixture too short to page)")
	}
	assertSelectionInView(t, m, "pgup")
}

// TestViewportFollowsPgDown is R136's row 2, down half. Failure against
// 2752c9e: starting flush at the bottom with the selection on the first
// session, one page down lands on session 5 while sidebarScroll stays at
// maxSidebarOffset (52) -- "pgdown: sidebarScroll = 52 leaves selection span
// [11,12] outside window [52,62)".
func TestViewportFollowsPgDown(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.selected = 0
	m.sidebarScroll = maxSidebarOffset(m)
	updated, _ := m.Update(key("pgdown"))
	m = updated.(Model)
	if m.selected == 0 {
		t.Fatalf("pgdown did not move the selection (fixture too short to page)")
	}
	assertSelectionInView(t, m, "pgdown")
}

// TestViewportFollowsG is R136's row 3, top half: `g` jumps to
// visibleSessionIndices()[0]. Failure against 2752c9e: starting flush at
// the bottom with the selection in the middle, `g` lands on session 0
// while sidebarScroll stays at maxSidebarOffset (52) -- "g: sidebarScroll = 52
// leaves selection span [1,2] outside window [52,62)".
func TestViewportFollowsG(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.selected = 15
	m.sidebarScroll = maxSidebarOffset(m)
	updated, _ := m.Update(key("g"))
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("g landed on %d, want 0", m.selected)
	}
	assertSelectionInView(t, m, "g")
}

// TestViewportFollowsCapitalG is R136's row 3, bottom half: `G` jumps to
// the last visible session. Failure against 2752c9e: starting flush at
// the top with the selection in the middle, `G` lands on session 29 while
// sidebarScroll stays 0 -- "G: sidebarScroll = 0 leaves selection span
// [59,60] outside window [0,10)".
func TestViewportFollowsCapitalG(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.selected = 15
	m.sidebarScroll = 0
	updated, _ := m.Update(key("G"))
	m = updated.(Model)
	if m.selected != 29 {
		t.Fatalf("G landed on %d, want 29", m.selected)
	}
	assertSelectionInView(t, m, "G")
}

// TestViewportFollowsSpace is R136's row 4: the attention walk
// (nextAttentionSelection). Only session 29 needs attention ("waiting");
// starting from session 0 ("idle", no attention) with sidebarScroll flush
// at the top, space must walk all the way to session 29. Failure against
// 2752c9e: sidebarScroll stays 0 while the selection lands on 29 --
// "space: sidebarScroll = 0 leaves selection span [59,60] outside window
// [0,10)".
func TestViewportFollowsSpace(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.sessions[29].Status = "waiting"
	m.baseSessions = m.sessions
	m.selected = 0
	m.sidebarScroll = 0
	updated, _ := m.Update(key(" "))
	m = updated.(Model)
	if m.selected != 29 {
		t.Fatalf("space landed on %d, want 29", m.selected)
	}
	assertSelectionInView(t, m, "space")
}

// viewportFollowCollapseTestModel builds two real groups with DISTINCT
// durable GroupIDs (task 008 gotcha: sessionGroupID falls back to the
// shared sentinel 0 for every session whose store.Session.GroupID is nil,
// so two groups told apart only by GroupName -- as selectionSeamTestModel
// builds them -- collapse and expand TOGETHER; c's own test needs group
// "a" collapsible independently of group "b").
func viewportFollowCollapseTestModel(nA, nB, height int) Model {
	idA, idB := int64(1), int64(2)
	var sessions []store.Session
	for i := 0; i < nA; i++ {
		sessions = append(sessions, store.Session{ID: fmt.Sprintf("a%02d", i), Name: fmt.Sprintf("a%02d", i), CWD: "/work/a", GroupName: "a", GroupID: &idA})
	}
	for i := 0; i < nB; i++ {
		sessions = append(sessions, store.Session{ID: fmt.Sprintf("b%02d", i), Name: fmt.Sprintf("b%02d", i), CWD: "/work/b", GroupName: "b", GroupID: &idB})
	}
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	return m
}

// TestViewportFollowsC is R136's row 5: `c` collapses the selected
// session's own group (task 013's toggleGroupCollapse), which relocates
// m.selected to the nearest still-visible session BEFORE tui.go's own `c`
// handler re-runs the follow via setSelection(m.selected). The fixture
// splits into group "a" (sessions 0-2) and group "b" (sessions 3-29):
// selecting a session inside "b" and collapsing it relocates the cursor
// backward onto session 2 (the last member of "a", still visible).
// Failure against 2752c9e: seeded flush at the bottom of the UNcollapsed
// list, sidebarScroll stays there (now far past the end of the much
// shorter collapsed list) while the selection lands on session 2 --
// "c: sidebarScroll = 53 leaves selection span [5,6] outside window
// [53,63)".
func TestViewportFollowsC(t *testing.T) {
	m := viewportFollowCollapseTestModel(3, 27, 13)
	m.selected = 15
	m.sidebarScroll = maxSidebarOffset(m)
	updated, _ := m.Update(key("c"))
	m = updated.(Model)
	if m.selected != 2 {
		t.Fatalf("c relocated the cursor to %d, want 2 (last member of group a, the nearest still-visible session)", m.selected)
	}
	assertSelectionInView(t, m, "c")
}

// TestViewportFollowsFilterQueryEdit is R136's row 6: a `/` query edit
// (filter.go's updateFilter, both the default typing case and the esc
// case call m.setSelection(m.nearestVisibleSelection(m.selected))).
// Selecting the LAST "drop" session (29) and then typing a query that
// narrows the list down to the 20 "keep" sessions clamps that stale index
// onto the last KEEP session (19) -- landing the cursor at the bottom of
// the newly-filtered, still-scrollable list. Failure against 2752c9e:
// sidebarScroll stays 0 (seeded flush at the top of the UNfiltered list)
// while the selection lands on the narrowed list's own last row --
// "/ query edit: sidebarScroll = 0 leaves selection span [39,40] outside
// window [0,9)".
func TestViewportFollowsFilterQueryEdit(t *testing.T) {
	m := viewportFollowFilterTestModel(20, 10, 13)
	m.selected = 29
	m.sidebarScroll = 0
	got, _ := m.Update(key("/"))
	m = got.(Model)
	if !m.filtering {
		t.Fatal("/ did not open the filter")
	}
	for _, r := range "keep" {
		got, _ = m.Update(key(string(r)))
		m = got.(Model)
	}
	if len(m.sessions) != 20 {
		t.Fatalf("query %q left %d sessions, want 20 (the keep group alone)", m.filterQuery, len(m.sessions))
	}
	if m.selected != 19 {
		t.Fatalf("selected = %d after filtering, want 19 (clamped onto the last keep session)", m.selected)
	}
	assertSelectionInView(t, m, "/ query edit")
}

// TestViewportWheelDriftIsNotPreservedAfterDown is R136's documented wheel
// independence (SPEC §11.8): the wheel scrolls without ever moving the
// selection, and may drift away from the cursor -- but the NEXT selection
// move must snap straight back, discarding the wheel's own offset rather
// than preserving it. Failure against 2752c9e: `down` assigns m.selected
// directly with no follow at all, so sidebarScroll is untouched by the
// keypress -- "sidebarScroll = 40 after down, unchanged from the drifted
// wheel offset 40 -- the wheel's offset was preserved instead of the cursor
// snapping back into view".
func TestViewportWheelDriftIsNotPreservedAfterDown(t *testing.T) {
	m := viewportFollowTestModel(30, 13)
	m.settings.Mouse = true
	m.selected = 0
	m.sidebarScroll = 0
	x, y := findRow(t, m, 0)
	for i := 0; i < 40; i++ {
		updated, _ := m.Update(wheelDown(x, y))
		m = updated.(Model)
	}
	if m.selected != 0 {
		t.Fatalf("the wheel changed the selection to %d, want 0 (wheel must never move the selection)", m.selected)
	}
	driftedScroll := m.sidebarScroll
	if driftedScroll == 0 {
		t.Fatalf("test setup: the wheel never moved sidebarScroll away from 0")
	}
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	start, _, _, _ := selectionSpan(entries, 0)
	if start >= driftedScroll && start < driftedScroll+contentHeight {
		t.Fatalf("test setup: selection 0 is still inside the drifted window [%d,%d)", driftedScroll, driftedScroll+contentHeight)
	}

	updated, _ := m.Update(key("down"))
	m = updated.(Model)
	if m.selected != 1 {
		t.Fatalf("down after wheel drift moved the selection to %d, want 1", m.selected)
	}
	if m.sidebarScroll == driftedScroll {
		t.Fatalf("sidebarScroll = %d after down, unchanged from the drifted wheel offset %d -- the wheel's offset was preserved instead of the cursor snapping back into view", m.sidebarScroll, driftedScroll)
	}
	assertSelectionInView(t, m, "wheel-away-then-down")
}

// TestViewportFlushDegradesAt80x24WithNoBlankTail is R136's 80x24 floor
// requirement: "degrade to flush rather than fighting the clamp, and never
// produce a negative offset or blank space below the last line". `G` at
// the supported minimum, on a list much taller than the sidebar, must land
// exactly at clampSidebarScroll's own flush bound with no padding
// (sidebarLineOther) entry trailing the real content. Failure against
// 2752c9e: sidebarScroll stays 0 (its zero value) instead of the flush
// bound -- "sidebarScroll = 0 after G at 80x24, want flush at 41 (no room to
// fight the clamp at this floor)".
func TestViewportFlushDegradesAt80x24WithNoBlankTail(t *testing.T) {
	m := viewportFollowTestModel(30, MinRows)
	layout := m.computeLayout()
	if layout.BelowMinimum {
		t.Fatalf("test setup: 80x%d unexpectedly BelowMinimum", MinRows)
	}
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	if contentHeight >= len(entries) {
		t.Fatalf("test setup: 80x%d does not scroll: contentHeight %d >= %d entries", MinRows, contentHeight, len(entries))
	}
	maxOffset := len(entries) - contentHeight

	updated, _ := m.Update(key("G"))
	m = updated.(Model)

	if m.sidebarScroll != maxOffset {
		t.Fatalf("sidebarScroll = %d after G at 80x24, want flush at %d (no room to fight the clamp at this floor)", m.sidebarScroll, maxOffset)
	}
	visible := m.sidebarVisibleEntries(contentWidth, contentHeight)
	if last := visible[len(visible)-1]; last.kind == sidebarLineOther {
		t.Fatalf("last visible line at 80x24 is blank padding, want the sidebar flush against its own last real entry")
	}
}
