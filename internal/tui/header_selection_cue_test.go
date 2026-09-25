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
// cursor is an addressable visual stop") over SPEC §11, REWRITTEN for
// task 003/R141 (GH #39, amended SPEC §11: "a header starts at the
// panel's first content column ... it reserves no gutter"). The original
// cue (cure-01-02) painted a literal "> " glyph into a 2-column gutter
// every header reserved whether or not it was selected; task 003 drops
// that gutter entirely (a header's sidebarEntry.gutter is now always "",
// so its chevron lands in the exact same column session rows' text does,
// level with "socket:") and moves the cursor cue onto two ATTRIBUTES
// instead of a glyph: the `selection`/`selection_idle` background across
// the header's own sidebarEntry, plus reverse video (SGR 7) over the
// header's rendered TEXT -- what survives NO_COLOR/ascii now that no
// literal glyph remains.
//
// headerRenderedLine below reproduces exactly the render path
// (sidebarEntries -> sidebarContentLine, no hand-picked width) so this
// suite's assertions exercise the same composition the fix touches.

// headerRenderedLine renders group groupID's own header entry the same way
// the real frame does: sidebarEntries builds the entry (text, gutter, bg),
// sidebarContentLine paints it. Returns both the painted line and the raw
// (pre-stripANSI) version, since the reverse-video attribute this task
// adds is only visible in the raw escapes -- stripANSI removes it along
// with every other SGR sequence. Fails the test outright if groupID has no
// header entry at this width, rather than silently comparing empty
// strings.
func headerRenderedLine(t *testing.T, m Model, groupID int64) (painted, raw string) {
	t.Helper()
	for _, e := range m.sidebarEntries(60) {
		if e.kind == sidebarLineHeader && e.groupID == groupID {
			raw = m.sidebarContentLine(64, e.gutter, e.text, e.bg)
			return stripANSI(raw), raw
		}
	}
	t.Fatalf("no header entry for group %d at width 60", groupID)
	return "", ""
}

// headerEntry fetches groupID's own sidebarEntry straight from
// sidebarEntries, for assertions that need the entry's raw fields
// (gutter, bg, text) rather than the fully-painted line.
func headerEntry(t *testing.T, m Model, groupID int64) sidebarEntry {
	t.Helper()
	for _, e := range m.sidebarEntries(60) {
		if e.kind == sidebarLineHeader && e.groupID == groupID {
			return e
		}
	}
	t.Fatalf("no header entry for group %d at width 60", groupID)
	return sidebarEntry{}
}

// headerHasCue reports whether groupID's own header entry carries the
// selection cue by EITHER of its two surviving forms: a non-empty
// background token (the colour half) or a reverse-video escape (SGR 7) in
// its own rendered text (the NO_COLOR/ascii-surviving half) -- mirroring
// headerSelectionCue's own two return values (bg token, selected bool).
func headerHasCue(t *testing.T, m Model, groupID int64) bool {
	t.Helper()
	e := headerEntry(t, m, groupID)
	if e.bg != theme.Token("") {
		return true
	}
	return strings.Contains(e.text, "\x1b[7m")
}

// TestHeaderGutterIsAlwaysEmptyRegardlessOfSelection proves task 003/R141
// success criterion 1: a header's sidebarEntry.gutter is always "" --
// selected or not -- since the whole point of dropping the old 2-column
// "> " gutter is that a header never reserves one at all.
func TestHeaderGutterIsAlwaysEmptyRegardlessOfSelection(t *testing.T) {
	idA, idB := int64(1), int64(2)
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{
		{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "b0", Name: "b0", Status: "idle", GroupName: "b", GroupID: &idB},
	}

	m.selected = headerCursor(idA)
	if got := headerEntry(t, m, idA).gutter; got != "" {
		t.Fatalf("selected header a's gutter = %q, want \"\" (task 003/R141: headers reserve no gutter)", got)
	}
	if got := headerEntry(t, m, idB).gutter; got != "" {
		t.Fatalf("unselected header b's gutter = %q, want \"\"", got)
	}
}

// TestHeaderChevronSitsInSameColumnAsSocketLine proves task 003/R141
// success criterion 1's other half: with no gutter reserved, a header's
// chevron lands in the exact same column "socket:" does -- in colour,
// NO_COLOR and ascii modes alike, since none of the three changes whether
// a gutter is reserved.
func TestHeaderChevronSitsInSameColumnAsSocketLine(t *testing.T) {
	idA := int64(1)
	for _, tc := range []struct {
		name     string
		settings config.Settings
	}{
		{"colour", config.Settings{Socket: "deck", Color: true}},
		{"NO_COLOR", config.Settings{Socket: "deck", Color: false}},
		{"ascii", config.Settings{Socket: "deck", Color: true, ASCII: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, tc.settings, "")
			m.sessions = []store.Session{
				{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
			}

			var socketEntry sidebarEntry
			haveSocket := false
			var header sidebarEntry
			haveHeader := false
			for _, e := range m.sidebarEntries(60) {
				if e.kind == sidebarLineOther && strings.HasPrefix(e.text, "socket:") {
					socketEntry, haveSocket = e, true
				}
				if e.kind == sidebarLineHeader && e.groupID == idA {
					header, haveHeader = e, true
				}
			}
			if !haveSocket || !haveHeader {
				t.Fatalf("test setup: haveSocket=%v haveHeader=%v", haveSocket, haveHeader)
			}
			if socketEntry.gutter != "" || header.gutter != "" {
				t.Fatalf("socket gutter = %q, header gutter = %q, want both \"\"", socketEntry.gutter, header.gutter)
			}

			socketLine := stripANSI(m.sidebarContentLine(64, socketEntry.gutter, socketEntry.text, socketEntry.bg))
			headerLine := stripANSI(m.sidebarContentLine(64, header.gutter, header.text, header.bg))
			chevron := m.glyph("\u25be", "v")

			socketCol := strings.Index(socketLine, "socket:")
			headerCol := strings.Index(headerLine, chevron)
			if socketCol < 0 {
				t.Fatalf("socket line %q does not contain %q", socketLine, "socket:")
			}
			if headerCol < 0 {
				t.Fatalf("header line %q does not contain the chevron %q", headerLine, chevron)
			}
			if socketCol != headerCol {
				t.Fatalf("%s: socket: starts at column %d, header chevron starts at column %d, want them equal (task 003/R141: no gutter separates them)", tc.name, socketCol, headerCol)
			}
		})
	}
}

// TestHeaderCursorCueUnderColourIsSelectionBackground proves success
// criterion 2's colour half: a cursored header's sidebarEntry carries
// exactly m.sidebarSelectionToken() ("selection", sidebar focused) as its
// background, and an unselected header carries none.
func TestHeaderCursorCueUnderColourIsSelectionBackground(t *testing.T) {
	idA, idB := int64(1), int64(2)
	m := New(nil, config.Settings{Color: true}, "")
	m.sessions = []store.Session{
		{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "b0", Name: "b0", Status: "idle", GroupName: "b", GroupID: &idB},
	}
	m.selected = headerCursor(idA)

	selected := headerEntry(t, m, idA)
	if selected.bg != theme.Selection {
		t.Fatalf("selected header's bg = %q, want %q", selected.bg, theme.Selection)
	}
	// task 003/R141 dropped the old "> " gutter glyph the background cue
	// used to ride alongside (cure-01-02): the background alone must now
	// carry the cue, with no gutter reserved either way. Pinned separately
	// so this test cannot pass merely because the pre-task-003 background
	// already matched -- the old gutter glyph is exactly what must be gone.
	if selected.gutter != "" {
		t.Fatalf("selected header's gutter = %q, want \"\" (task 003/R141: no gutter, background alone carries the cue)", selected.gutter)
	}
	unselected := headerEntry(t, m, idB)
	if unselected.bg != theme.Token("") {
		t.Fatalf("unselected header's bg = %q, want \"\"", unselected.bg)
	}
	if unselected.gutter != "" {
		t.Fatalf("unselected header's gutter = %q, want \"\"", unselected.gutter)
	}
}

// TestHeaderCursorCueUnderNoColorIsReverseVideo proves success criterion
// 2's monochrome half: a cursored header's rendered text carries the SGR
// 7 (reverse video) escape under NO_COLOR (Color: false), and an
// unselected header carries neither it nor any other escape.
func TestHeaderCursorCueUnderNoColorIsReverseVideo(t *testing.T) {
	idA, idB := int64(1), int64(2)
	m := New(nil, config.Settings{Color: false}, "")
	m.sessions = []store.Session{
		{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
		{ID: "b0", Name: "b0", Status: "idle", GroupName: "b", GroupID: &idB},
	}
	m.selected = headerCursor(idA)

	selected := headerEntry(t, m, idA)
	if !strings.Contains(selected.text, "\x1b[7m") {
		t.Fatalf("selected header's text = %q, want it to carry the reverse-video escape \\x1b[7m", selected.text)
	}
	if !strings.Contains(selected.text, "\x1b[27m") {
		t.Fatalf("selected header's text = %q, want it to close reverse video with \\x1b[27m", selected.text)
	}
	unselected := headerEntry(t, m, idB)
	if strings.Contains(unselected.text, "\x1b[7m") {
		t.Fatalf("unselected header's text = %q, want no reverse-video escape", unselected.text)
	}
}

// TestHeaderCursorLeavesWidthAndLeftColumnUnchanged proves success
// criterion 2's last clause: painting the cursor cue never resizes or
// reindents the header -- the rendered width and the column its own
// chevron starts at are identical whether or not the cursor sits on it,
// since the cue is carried entirely by attributes (background, reverse
// video), never by a glyph or a gutter that could grow the line.
func TestHeaderCursorLeavesWidthAndLeftColumnUnchanged(t *testing.T) {
	idA := int64(1)
	m := New(nil, config.Settings{Color: true}, "")
	m.sessions = []store.Session{
		{ID: "a0", Name: "a0", Status: "idle", GroupName: "a", GroupID: &idA},
	}

	m.selected = sidebarCursor{}
	unselectedPainted, _ := headerRenderedLine(t, m, idA)

	m.selected = headerCursor(idA)
	selectedPainted, _ := headerRenderedLine(t, m, idA)

	if stringWidth(unselectedPainted) != stringWidth(selectedPainted) {
		t.Fatalf("rendered width changed with the cursor on/off: unselected %d, selected %d", stringWidth(unselectedPainted), stringWidth(selectedPainted))
	}
	chevron := m.glyph("\u25be", "v")
	unselectedCol := strings.Index(unselectedPainted, chevron)
	selectedCol := strings.Index(selectedPainted, chevron)
	if unselectedCol < 0 || selectedCol < 0 {
		t.Fatalf("chevron missing from a rendered line: unselected=%q selected=%q", unselectedPainted, selectedPainted)
	}
	if unselectedCol != selectedCol {
		t.Fatalf("chevron's left column moved with the cursor: unselected column %d, selected column %d", unselectedCol, selectedCol)
	}
	// task 003/R141: a header reserves no gutter at all, either selected or
	// not -- the old 2-column "> "/"  " gutter pair already left the
	// chevron's column consistent between the two states without dropping
	// the gutter, so the identical-column check above cannot by itself
	// distinguish the two behaviours; the gutter field itself must.
	m.selected = sidebarCursor{}
	if got := headerEntry(t, m, idA).gutter; got != "" {
		t.Fatalf("unselected header's gutter = %q, want \"\" (task 003/R141: no gutter reserved)", got)
	}
	m.selected = headerCursor(idA)
	if got := headerEntry(t, m, idA).gutter; got != "" {
		t.Fatalf("selected header's gutter = %q, want \"\" (task 003/R141: no gutter reserved)", got)
	}

	// The cue itself (background token, reverse video) lives entirely in
	// SGR escapes that stripANSI removes -- so the visible TEXT is expected
	// to be identical above; it is the RAW (unstripped) rendering that must
	// differ, which is what proves the cue actually painted something.
	m.selected = sidebarCursor{}
	_, unselectedRaw := headerRenderedLine(t, m, idA)
	m.selected = headerCursor(idA)
	_, selectedRaw := headerRenderedLine(t, m, idA)
	if unselectedRaw == selectedRaw {
		t.Fatalf("raw rendering did not change with the cursor on/off; want the selection cue's own SGR escapes to differ")
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
			hasCue := headerHasCue(t, m, id)
			if id == want && !hasCue {
				t.Fatalf("step %d: header %d is selected but carries no cue", step, id)
			}
			if id != want && hasCue {
				t.Fatalf("step %d: header %d is NOT selected but carries a cue", step, id)
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
		if !headerHasCue(t, m, idA) {
			t.Fatalf("key %q: header a lost its selection cue", k)
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
		if !headerHasCue(t, m, idB) {
			t.Fatalf("all-folded sidebar: selected header b carries no cue")
		}
		if headerHasCue(t, m, idA) {
			t.Fatalf("all-folded sidebar: unselected header a carries a cue")
		}
	})

	t.Run("empty groups", func(t *testing.T) {
		const emptyGroupID = int64(42)
		m := New(nil, config.Settings{}, "")
		m.allGroups = []store.Group{{ID: emptyGroupID, Name: "empty"}}
		m.selected = headerCursor(emptyGroupID)
		if !headerHasCue(t, m, emptyGroupID) {
			t.Fatalf("empty-group sidebar: selected header carries no cue")
		}
		if headerHasCue(t, m, 0) {
			t.Fatalf("empty-group sidebar: unselected default header carries a cue")
		}
	})
}
