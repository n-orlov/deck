package tui

import (
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// sidebarStripeTestModel is a colour-enabled model with three sessions,
// wide enough that nothing wraps, and NOTHING selected (m.selected = -1,
// an index no session row can ever equal) so the stripe's own background
// is visible on every row rather than being masked by the selection
// focus cue on whichever row m.selected would otherwise land on.
func sidebarStripeTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	// A real, current CreatedAt (task 012/R120: line 2's age is now bare, no
	// "created " label) rather than a fixed epoch: the assertion below needs
	// "just now" as a stable anchor, not a day count that drifts with
	// wall-clock time.
	now := time.Now().UnixMilli()
	m.sessions = []store.Session{
		{ID: "s1", Name: "one", Agent: "shell", Status: "running", CreatedAt: now},
		{ID: "s2", Name: "two", Agent: "shell", Status: "running", CreatedAt: now},
		{ID: "s3", Name: "three", Agent: "shell", Status: "running", CreatedAt: now},
	}
	m.selected = -1
	return m
}

// TestSidebarStripeAlternatesPerSessionBlockNotPerLine proves task 084:
// three consecutive sessions must show background/surface/background (or
// the reverse), never all-same and never a phase that flips mid-session --
// three sessions is the minimum that makes an off-by-one in the phase
// visible (two sessions cannot distinguish "alternates" from "every row
// its own phase" or "always the same phase" as reliably as three can).
func TestSidebarStripeAlternatesPerSessionBlockNotPerLine(t *testing.T) {
	m := sidebarStripeTestModel(t)
	surfaceHex := tokenHex(t, m, theme.Surface)
	backgroundHex := tokenHex(t, m, theme.Background)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	rowOne := findRowContaining(t, term, "one")
	rowTwo := findRowContaining(t, term, "two")
	rowThree := findRowContaining(t, term, "three")

	bg1, ok1 := cellBgHex(t, term, findCol(t, term, rowOne, "one"), rowOne)
	bg2, ok2 := cellBgHex(t, term, findCol(t, term, rowTwo, "two"), rowTwo)
	bg3, ok3 := cellBgHex(t, term, findCol(t, term, rowThree, "three"), rowThree)

	// Task 004/R118: every row now carries SOME background (its stripe
	// phase, theme.Surface, when the row's bg override is empty resolves
	// through theme.Background instead of falling through to the
	// terminal's own) -- ok is expected true unconditionally now, unlike
	// before this task.
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("every sidebar row must carry a background now that deck paints its own canvas (task 004/R118): ok1=%v ok2=%v ok3=%v", ok1, ok2, ok3)
	}

	// Sessions 1 and 3 must agree with each other and disagree with
	// session 2 -- exactly the pattern a period-2 alternation produces
	// and an off-by-one (e.g. every row alternating instead of every
	// session block) would not.
	if bg1 != bg3 {
		t.Fatalf("sessions 1 and 3 should share a stripe phase: session1 bg=%q session3 bg=%q", bg1, bg3)
	}
	if bg1 == bg2 {
		t.Fatalf("session 2 should be on the OPPOSITE stripe phase from sessions 1/3, but all three matched: bg=%q", bg1)
	}
	// Exactly one phase paints theme.Surface; the other paints the plain
	// theme.Background (task 004/R118 -- previously "nothing", falling
	// through to the panel's own background).
	switch {
	case bg1 == surfaceHex && bg2 == backgroundHex:
	case bg1 == backgroundHex && bg2 == surfaceHex:
	default:
		t.Fatalf("stripe phases must be exactly {theme.Surface, theme.Background}, got session1=%q session2=%q (surface=%s background=%s)", bg1, bg2, surfaceHex, backgroundHex)
	}

	// Line 2 (task 012/R120: a bare age, no "created " label) of the SAME
	// session must match line 1's own phase -- both lines of one session
	// block share one phase, never each choosing its own.
	rowOneLine2 := rowOne + 1
	rowTwoLine2 := rowTwo + 1
	bg1line2, ok1line2 := cellBgHex(t, term, findCol(t, term, rowOneLine2, "just now"), rowOneLine2)
	bg2line2, ok2line2 := cellBgHex(t, term, findCol(t, term, rowTwoLine2, "just now"), rowTwoLine2)
	if !ok1line2 || bg1line2 != bg1 {
		t.Fatalf("session 1's line 2 background = (%q,%v), want it to match line 1's (%q,%v)", bg1line2, ok1line2, bg1, ok1)
	}
	if !ok2line2 || bg2line2 != bg2 {
		t.Fatalf("session 2's line 2 background = (%q,%v), want it to match line 1's (%q,%v)", bg2line2, ok2line2, bg2, ok2)
	}
}

// TestSidebarStripeSelectionWinsRegardlessOfPhase proves theme.Selection
// still wins over the stripe on whichever row is currently selected,
// whether that row's own phase would otherwise have painted
// theme.Surface or nothing.
func TestSidebarStripeSelectionWinsRegardlessOfPhase(t *testing.T) {
	m := sidebarStripeTestModel(t)
	selectionHex := tokenHex(t, m, theme.Selection)
	surfaceHex := tokenHex(t, m, theme.Surface)
	if selectionHex == surfaceHex {
		t.Skip("this theme's selection and surface tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	// Select session index 1 ("two"), whichever stripe phase it lands on.
	m.selected = 1
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "two")
	bg, ok := cellBgHex(t, term, findCol(t, term, row, "two"), row)
	if !ok {
		t.Fatalf("selected row has no background at all, want theme.Selection")
	}
	if bg != selectionHex {
		t.Fatalf("selected row background = %s, want theme.Selection %s", bg, selectionHex)
	}
}

// TestSidebarStripeAbsentUnderNoColor proves NO_COLOR/DECK_COLOR=0
// (m.settings.Color == false here) degrades to today's appearance with
// the stripe explicitly absent: no background anywhere in the sidebar,
// on either phase.
func TestSidebarStripeAbsentUnderNoColor(t *testing.T) {
	m := New(nil, config.Settings{Color: false}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "one", Agent: "shell", Status: "running", CreatedAt: 1000},
		{ID: "s2", Name: "two", Agent: "shell", Status: "running", CreatedAt: 1000},
		{ID: "s3", Name: "three", Agent: "shell", Status: "running", CreatedAt: 1000},
	}
	m.selected = -1

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	for _, name := range []string{"one", "two", "three"} {
		row := findRowContaining(t, term, name)
		if _, ok := cellBgHex(t, term, findCol(t, term, row, name), row); ok {
			t.Fatalf("session %q has a background under Color=false, want the stripe absent entirely", name)
		}
	}
}

// TestSidebarStripeHeaderNeverParticipates proves that with
// [ui] group_by_workspace on, a workspace header row never gets the
// stripe's theme.Surface background, and that the per-session phase
// counter continues across a group boundary rather than resetting (task
// 084's own stated choice, recorded here as an executable pin: session 3
// -- the first session of the SECOND group -- continues the alternation
// from the two sessions in the first group rather than restarting it).
func TestSidebarStripeHeaderNeverParticipates(t *testing.T) {
	m := New(nil, config.Settings{Color: true, GroupByWorkspace: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "one", Agent: "shell", Status: "running", Workspace: "wsA", CreatedAt: 1000},
		{ID: "s2", Name: "two", Agent: "shell", Status: "running", Workspace: "wsA", CreatedAt: 1000},
		{ID: "s3", Name: "three", Agent: "shell", Status: "running", Workspace: "wsB", CreatedAt: 1000},
	}
	m.selected = -1
	surfaceHex := tokenHex(t, m, theme.Surface)
	backgroundHex := tokenHex(t, m, theme.Background)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	headerRow := findRowContaining(t, term, "wsA")
	// Task 004/R118: a header row never takes the stripe's theme.Surface,
	// but it is no longer left with no background at all either -- it
	// resolves through the same "" -> theme.Background fallback every
	// other non-row sidebar line now does (sidebarContentLine).
	if bg, ok := cellBgHex(t, term, findCol(t, term, headerRow, "wsA"), headerRow); !ok || bg != backgroundHex {
		t.Fatalf("workspace header row background = (%q,%v), want theme.Background %s (headers never take the stripe, but still paint the canvas)", bg, ok, backgroundHex)
	}

	rowOne := findRowContaining(t, term, "one")
	rowTwo := findRowContaining(t, term, "two")
	rowThree := findRowContaining(t, term, "three")
	bg1, ok1 := cellBgHex(t, term, findCol(t, term, rowOne, "one"), rowOne)
	bg2, ok2 := cellBgHex(t, term, findCol(t, term, rowTwo, "two"), rowTwo)
	bg3, ok3 := cellBgHex(t, term, findCol(t, term, rowThree, "three"), rowThree)
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("every session row must carry a background now that deck paints its own canvas (task 004/R118): ok1=%v ok2=%v ok3=%v", ok1, ok2, ok3)
	}

	if bg1 == bg2 {
		t.Fatalf("session 1 and session 2 (same group, consecutive positions) should be on opposite phases, both got %q", bg1)
	}
	// Session 3 is the first session of the SECOND group, in the third
	// counted session position overall (0-indexed position 2, an odd
	// index) -- it must match session 1's phase (also an even/odd
	// counterpart two apart), proving the header between them did not
	// reset or advance the counter.
	if bg1 != bg3 {
		t.Fatalf("session 3 (first of the second group) should share session 1's phase (counter continues across the header), got session1=%q session3=%q", bg1, bg3)
	}
	// Exactly one of session 1/2's phases paints theme.Surface; the other
	// paints the plain theme.Background (task 004/R118).
	switch {
	case bg1 == surfaceHex && bg2 == backgroundHex:
	case bg1 == backgroundHex && bg2 == surfaceHex:
	default:
		t.Fatalf("stripe phases must be exactly {theme.Surface, theme.Background}, got session1=%q session2=%q (surface=%s background=%s)", bg1, bg2, surfaceHex, backgroundHex)
	}
}
