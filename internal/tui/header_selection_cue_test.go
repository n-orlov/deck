package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is cure-01-02's own regression coverage for review findings
// R136 ("selection always visibly on screen") and R137 ("the header
// cursor is an addressable visual stop") over SPEC §11: a header cursor
// existed (task 012/D.1) but painted no visible cue at all --
// TestReviewHeaderCursorHasVisibleSidebarCue (review_phase4c_test.go:33)
// proved it by rendering the sidebar with m.selected on header a, then on
// header b, and finding every painted line (text, gutter AND background)
// byte-identical between the two: "neither header has a selection cue".
// headerRenderedLine below reproduces exactly that render path
// (sidebarEntries -> sidebarContentLine, no hand-picked width) so this
// suite's own fail-before evidence and its green re-check use the same
// composition the fix touches.
//
// Fail-before (stashed group.go/tui.go, only this test file kept, ci/run.sh
// go test -run '^TestHeaderSelectionCueVisibleUnderColorAndMonochrome$'
// -v ./internal/tui/):
//
//	header_selection_cue_test.go:75: colored: moving the cursor from header
//	a to header b left the fully-painted line byte-identical; want a
//	visible selection cue
//	header_selection_cue_test.go:83: colored: header a's own line did not
//	change between being selected and not; want a visible selection cue
//	header_selection_cue_test.go:96: monochrome: moving the cursor from
//	header a to header b left the fully-painted line byte-identical
//	(NO_COLOR strips background alone -- SPEC requires the cue survive
//	monochrome too)
//
// which is exactly review's own finding, restated as a fail against the
// unfixed tree rather than only cited from probes.log.

// headerRenderedLine renders group groupID's own header entry the same way
// the real frame does: sidebarEntries builds the entry (text, gutter, bg),
// sidebarContentLine paints it, stripANSI leaves only what a terminal
// actually shows. Fails the test outright if groupID has no header entry
// at this width, rather than silently comparing empty strings.
func headerRenderedLine(t *testing.T, m Model, groupID int64) string {
	t.Helper()
	for _, e := range m.sidebarEntries(60) {
		if e.kind == sidebarLineHeader && e.groupID == groupID {
			return stripANSI(m.sidebarContentLine(64, e.gutter, e.text, e.bg))
		}
	}
	t.Fatalf("no header entry for group %d at width 60", groupID)
	return ""
}

// headerGutterAndBG returns groupID's own raw (pre-stripANSI) gutter and
// background token, straight off the sidebarEntry -- rendered strings hide
// which of the two carried the difference; the assertions below need to
// know that a background token flip alone is never enough (canvasBackground
// drops it outright under NO_COLOR/Color:false, panel.go's own documented
// "leaves parts untouched" case) and check the gutter GLYPH separately for
// exactly that reason.
func headerGutterAndBG(t *testing.T, m Model, groupID int64) (string, theme.Token) {
	t.Helper()
	for _, e := range m.sidebarEntries(60) {
		if e.kind == sidebarLineHeader && e.groupID == groupID {
			return e.gutter, e.bg
		}
	}
	t.Fatalf("no header entry for group %d at width 60", groupID)
	return "", ""
}

// TestHeaderSelectionCueVisibleUnderColorAndMonochrome is
// TestReviewHeaderCursorHasVisibleSidebarCue's own finding, run against
// both a colour-enabled and a colour-disabled (NO_COLOR/DECK_COLOR=0)
// model -- SPEC §11 requires the cue survive both, and a background-only
// cue would only ever survive the first.
func TestHeaderSelectionCueVisibleUnderColorAndMonochrome(t *testing.T) {
	for _, tc := range []struct {
		name  string
		color bool
	}{
		{"colored", true},
		{"monochrome", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idA, idB := int64(1), int64(2)
			m := New(nil, config.Settings{Color: tc.color}, "")
			m.sessions = []store.Session{
				{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
				{ID: "b0", Name: "b0", Status: "idle", GroupName: "b", GroupID: &idB},
			}

			m.selected = headerCursor(idA)
			aSelected := headerRenderedLine(t, m, idA)
			bWhenAIsSelected := headerRenderedLine(t, m, idB)

			m.selected = headerCursor(idB)
			aWhenBIsSelected := headerRenderedLine(t, m, idA)
			bSelected := headerRenderedLine(t, m, idB)

			if aSelected == bWhenAIsSelected {
				t.Fatalf("%s: header a (selected) and header b (not) rendered byte-identical lines %q; want a visible selection cue", tc.name, aSelected)
			}
			if aSelected == aWhenBIsSelected {
				t.Fatalf("%s: header a's own line did not change between being selected (%q) and not (%q); want a visible selection cue", tc.name, aSelected, aWhenBIsSelected)
			}
			if bSelected == bWhenAIsSelected {
				t.Fatalf("%s: header b's own line did not change between being selected (%q) and not (%q); want a visible selection cue", tc.name, bSelected, bWhenAIsSelected)
			}

			// The monochrome case specifically needs the GUTTER (a literal
			// glyph, never only a colour) to carry the cue, since a
			// background-only cue is invisible once colour is stripped.
			if !tc.color {
				selGutter, _ := headerGutterAndBG(t, m, idB)
				unselGutter, _ := headerGutterAndBG(t, m, idA)
				if !strings.Contains(selGutter, ">") {
					t.Fatalf("monochrome: selected header's own gutter = %q, want it to carry a literal cue glyph", selGutter)
				}
				if strings.Contains(unselGutter, ">") {
					t.Fatalf("monochrome: unselected header's own gutter = %q, want no cue glyph", unselGutter)
				}
			}
		})
	}
}

// TestHeaderSelectionCueMovesWithKeyboardNavigation proves the cue tracks
// the cursor, not just that SOME header carries one: fold every group so
// the visible stops are exactly the three headers (visualOrder skips rows
// under a collapsed group), then walk "down" across all three, checking at
// each stop that exactly the current header carries the cue and neither
// neighbour does.
func TestHeaderSelectionCueMovesWithKeyboardNavigation(t *testing.T) {
	m, idA, idB, idC := foldUnfoldTestModel()
	for _, id := range []int64{idA, idB, idC} {
		m.setGroupCollapsed(id, true)
	}
	m.selected = headerCursor(idA)

	order := []int64{idA, idB, idC}
	for step, want := range order {
		if m.selected != headerCursor(want) {
			t.Fatalf("step %d: cursor = %+v, want header %d", step, m.selected, want)
		}
		for _, id := range order {
			gutter, bg := headerGutterAndBG(t, m, id)
			hasCue := strings.Contains(gutter, ">") || bg != theme.Token("")
			if id == want && !hasCue {
				t.Fatalf("step %d: header %d is selected but carries no cue (gutter=%q bg=%q)", step, id, gutter, bg)
			}
			if id != want && hasCue {
				t.Fatalf("step %d: header %d is NOT selected but carries a cue (gutter=%q bg=%q)", step, id, gutter, bg)
			}
		}
		if step < len(order)-1 {
			next, _ := m.Update(key("down"))
			m = next.(Model)
		}
	}
}

// TestHeaderSelectionCuePreservedAcrossCAndLeftRight proves the cue stays
// put through the exact keys SPEC §11/task 012 route through a header
// cursor without moving it: `c` (toggle fold), `left` (fold), `right`
// (unfold) -- none of them are supposed to move the cursor off the header
// they were pressed on, and now that the cue exists, none of them may
// drop it either.
func TestHeaderSelectionCuePreservedAcrossCAndLeftRight(t *testing.T) {
	m, idA, _, _ := foldUnfoldTestModel()
	m.selected = headerCursor(idA)

	for _, k := range []string{"c", "left", "right", "c"} {
		next, _ := m.Update(key(k))
		m = next.(Model)
		if m.selected != headerCursor(idA) {
			t.Fatalf("key %q moved the cursor to %+v, want it to stay on header a", k, m.selected)
		}
		gutter, bg := headerGutterAndBG(t, m, idA)
		if !strings.Contains(gutter, ">") && bg == theme.Token("") {
			t.Fatalf("key %q: header a lost its selection cue (gutter=%q bg=%q)", k, gutter, bg)
		}
	}
}

// TestHeaderSelectionCueOnAllFoldedAndEmptyGroupsSidebar proves the cue
// survives the two edge shapes this task's own successCriteria name
// explicitly: an all-folded sidebar (every group collapsed, only headers
// visible) and an empty-group sidebar (zero sessions, a defined-but-empty
// group still rendering per cure-01-02's own groupSessions() fix).
func TestHeaderSelectionCueOnAllFoldedAndEmptyGroupsSidebar(t *testing.T) {
	t.Run("all folded", func(t *testing.T) {
		m, idA, idB, idC := foldUnfoldTestModel()
		for _, id := range []int64{idA, idB, idC} {
			m.setGroupCollapsed(id, true)
		}
		m.selected = headerCursor(idB)
		gutter, bg := headerGutterAndBG(t, m, idB)
		if !strings.Contains(gutter, ">") && bg == theme.Token("") {
			t.Fatalf("all-folded sidebar: selected header b carries no cue (gutter=%q bg=%q)", gutter, bg)
		}
		other, otherBG := headerGutterAndBG(t, m, idA)
		if strings.Contains(other, ">") || otherBG != theme.Token("") {
			t.Fatalf("all-folded sidebar: unselected header a carries a cue (gutter=%q bg=%q)", other, otherBG)
		}
	})

	t.Run("empty groups", func(t *testing.T) {
		const emptyGroupID = int64(42)
		m := New(nil, config.Settings{}, "")
		m.allGroups = []store.Group{{ID: emptyGroupID, Name: "empty"}}
		m.selected = headerCursor(emptyGroupID)
		gutter, bg := headerGutterAndBG(t, m, emptyGroupID)
		if !strings.Contains(gutter, ">") && bg == theme.Token("") {
			t.Fatalf("empty-group sidebar: selected header carries no cue (gutter=%q bg=%q)", gutter, bg)
		}
		defaultGutter, defaultBG := headerGutterAndBG(t, m, 0)
		if strings.Contains(defaultGutter, ">") || defaultBG != theme.Token("") {
			t.Fatalf("empty-group sidebar: unselected default header carries a cue (gutter=%q bg=%q)", defaultGutter, defaultBG)
		}
	})
}
