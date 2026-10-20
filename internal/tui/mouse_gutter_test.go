package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R328/R329: the cursor gutter of a sidebar row is the two cells that hold
// the `>` marker (columns 2 and 3: border, one pad column, then the gutter).
// A press there navigates without ever attaching; a press elsewhere on the
// row, on empty sidebar space and on a group header keeps its old meaning.

const gutterTestX = sidebarGutterX // first gutter column

// gutterFixture is the real-store fixture with row-waiting backed by a live
// tmux pane (the only row an interactive entry can succeed on).
func gutterFixture(t *testing.T, attachOnClick bool) (Model, *store.Store, int, int) {
	t.Helper()
	m, db := inertFixture(t, SortOrderAttention, "row-idle", true)
	m = inertPane(t, m, db, "row-waiting")
	m.settings.AttachOnClick = attachOnClick
	order := inertOrder(m)
	waiting, other := -1, -1
	for i, id := range order {
		switch id {
		case "row-waiting":
			waiting = i
		case "row-running":
			other = i
		}
	}
	return m, db, waiting, other
}

// pressGutter and pressMain press the gutter cell / the middle of the row's
// line, both located through the one hit-test geometry.
func pressGutter(t *testing.T, m Model, index int) Model {
	t.Helper()
	_, y := findRow(t, m, index)
	if hit := m.hitTest(gutterTestX, y); !hit.gutter || hit.sessionIndex != index {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want a gutter hit on row %d", gutterTestX, y, hit, index)
	}
	updated, _ := m.Update(press(gutterTestX, y))
	return updated.(Model)
}

func pressMain(t *testing.T, m Model, index int) Model {
	t.Helper()
	x, y := findRow(t, m, index)
	if hit := m.hitTest(x, y); hit.gutter || hit.target != hitTargetRow {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want a non-gutter row hit", x, y, hit)
	}
	updated, _ := m.Update(press(x, y))
	return updated.(Model)
}

func enterOn(t *testing.T, m Model, index int) Model {
	t.Helper()
	m = pressMain(t, m, index)
	if !m.interactive {
		t.Fatalf("test setup: main-area click did not enter interactive mode: %q", m.attachError)
	}
	return m
}

// 1. While interactive, the gutter of the attached row leaves interactive.
func TestGutterClickOnAttachedRowLeavesInteractive(t *testing.T) {
	m, db, waiting, _ := gutterFixture(t, true)
	m = enterOn(t, m, waiting)
	before := snapshotAll(t, db)
	order := inertOrder(m)
	got := pressGutter(t, m, waiting)
	if got.interactive {
		t.Fatalf("gutter click on the attached row left interactive mode on")
	}
	if got.selected != rowCursor(waiting) {
		t.Fatalf("selection moved to %v, want it left on row %d", got.selected, waiting)
	}
	assertInert(t, "gutter detach", got, db, order, before)
}

// 2. While interactive, the gutter of another row selects it without
// entering interactive mode on it.
func TestGutterClickOnOtherRowSelectsWithoutEntering(t *testing.T) {
	m, db, waiting, other := gutterFixture(t, true)
	m = enterOn(t, m, waiting)
	got := pressGutter(t, m, other)
	if got.interactive {
		t.Fatalf("gutter click on another row entered or kept interactive mode")
	}
	if got.selected != rowCursor(other) {
		t.Fatalf("selected %v, want row %d", got.selected, other)
	}
	if got.attachError != "" || fmt.Sprint(got.entryRefusal) != fmt.Sprint(m.entryRefusal) {
		t.Fatalf("gutter navigate attempted an entry: attachError=%q refusal=%v", got.attachError, got.entryRefusal)
	}
	_ = db
}

// 3. A main-area click is unchanged, in list mode and while interactive.
func TestMainAreaClickIsUnchangedByTheGutter(t *testing.T) {
	m, _, waiting, other := gutterFixture(t, true)
	listClick := pressMain(t, m, waiting)
	if !listClick.interactive || listClick.selected != rowCursor(waiting) {
		t.Fatalf("list-mode main click: interactive=%v selected=%v, want select + enter", listClick.interactive, listClick.selected)
	}
	left, _ := listClick.exitInteractive() // release the pane's ownership claim
	_ = left
	live := enterOn(t, m, waiting)
	want, _ := live.retargetInteractiveSidebarClick(other)
	got := pressMain(t, live, other)
	w := want.(Model)
	if got.selected != w.selected || got.interactive != w.interactive || got.attachError != w.attachError ||
		fmt.Sprint(got.entryRefusal) != fmt.Sprint(w.entryRefusal) {
		t.Fatalf("interactive main click differs from the retarget path: got sel=%v int=%v, want sel=%v int=%v",
			got.selected, got.interactive, w.selected, w.interactive)
	}
	if got.selected != rowCursor(other) {
		t.Fatalf("main click selected %v, want row %d", got.selected, other)
	}
}

// 4. Empty sidebar space is unchanged: it leaves interactive mode (as Ctrl+Q)
// and does nothing in list mode.
func TestEmptySidebarClickIsUnchangedByTheGutter(t *testing.T) {
	m, _, waiting, _ := gutterFixture(t, true)
	x, y := findEmptySidebarSpace(t, m)
	updated, _ := m.Update(press(x, y))
	if g := updated.(Model); g.interactive || g.selected != m.selected {
		t.Fatalf("list-mode empty-space click changed state: interactive=%v selected=%v", g.interactive, g.selected)
	}
	live := enterOn(t, m, waiting)
	updated, _ = live.Update(press(x, y))
	if g := updated.(Model); g.interactive || g.selected != rowCursor(waiting) {
		t.Fatalf("interactive empty-space click: interactive=%v selected=%v, want left with selection kept", g.interactive, g.selected)
	}
}

// 5. The gutter is inert on group headers: a press on a header's gutter
// columns is the header click, collapsing the group and leaving interactive
// mode as it was.
func TestGutterColumnsOnAGroupHeaderKeepTheCollapseBehaviour(t *testing.T) {
	db := openInertStore(t)
	g, err := db.CreateGroup(t.Context(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha-one", Agent: "shell", Status: "running", Slug: "a1", GroupName: g.Name, GroupID: &g.ID},
	})
	m.store = db
	m.width, m.height = 100, 30
	m.selected = rowCursor(0)
	_, y := findHeader(t, m, "alpha")
	hit := m.hitTest(gutterTestX, y)
	if hit.target != hitTargetHeader || hit.gutter {
		t.Fatalf("hitTest on a header's gutter columns = %+v, want a plain header hit", hit)
	}
	updated, _ := m.Update(press(gutterTestX, y))
	got := updated.(Model)
	if !got.isGroupCollapsed(g.ID) {
		t.Fatalf("a press on a header's gutter columns did not collapse the group")
	}
	if got.interactive {
		t.Fatalf("a header press entered interactive mode")
	}
}

// 6 and 7. While not interactive a gutter click only selects, whatever
// attach_on_click says, and writes no seen state and moves no row (R320).
func TestGutterClickWhileNotInteractiveSelectsOnly(t *testing.T) {
	for _, attach := range []bool{true, false} {
		t.Run(fmt.Sprintf("attach_on_click=%v", attach), func(t *testing.T) {
			m, db, waiting, _ := gutterFixture(t, attach)
			before := snapshotAll(t, db)
			order := inertOrder(m)
			got := pressGutter(t, m, waiting)
			if got.interactive || got.attachError != "" {
				t.Fatalf("gutter click entered interactive mode (interactive=%v err=%q)", got.interactive, got.attachError)
			}
			if got.selected != rowCursor(waiting) {
				t.Fatalf("selected %v, want row %d", got.selected, waiting)
			}
			assertInert(t, "gutter select", got, db, order, before)
		})
	}
}

// R320: a gutter navigate from an attached row to an unseen waiting/error
// row leaves that row's position and seen state untouched, in the attention
// and the activity orders.
func TestGutterNavigateLeavesTheLandedRowUntouched(t *testing.T) {
	for _, order := range []string{SortOrderAttention, SortOrderActivity} {
		for _, landed := range []string{"row-waiting", "row-error"} {
			t.Run(order+"/"+landed, func(t *testing.T) {
				m, db := inertFixture(t, order, "row-idle", true)
				m = inertPane(t, m, db, "row-idle")
				ids := inertOrder(m)
				idx := func(id string) int {
					for i, s := range ids {
						if s == id {
							return i
						}
					}
					t.Fatalf("row %s missing", id)
					return -1
				}
				m = enterOn(t, m, idx("row-idle"))
				before := snapshotAll(t, db)
				if before[landed].Session.StatusSeen() {
					t.Fatalf("fixture: %s is already seen", landed)
				}
				got := pressGutter(t, m, idx(landed))
				if got.interactive || got.selected != rowCursor(idx(landed)) {
					t.Fatalf("navigate: interactive=%v selected=%v", got.interactive, got.selected)
				}
				assertInert(t, "gutter navigate", got, db, ids, before)
			})
		}
	}
}

// R329: the gutter is the two cells the `>` is drawn in, at the 80x24
// minimum and with ascii glyphs, side by side and stacked. The rendered
// frame and the hit-test agree on the cells.
func TestGutterGeometryMatchesTheRenderedMarker(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		for _, size := range [][2]int{{80, 24}, {60, 30}} {
			t.Run(fmt.Sprintf("ascii=%v/%dx%d", ascii, size[0], size[1]), func(t *testing.T) {
				m := New(nil, config.Settings{Mouse: true, AttachOnClick: true, ASCII: ascii}, "")
				m.width, m.height = size[0], size[1]
				m.sessions = []store.Session{
					{ID: "s1", Name: "gutx", Agent: "shell", Status: "running", Slug: "s1", CreatedAt: 1000},
					{ID: "s2", Name: "guty", Agent: "shell", Status: "running", Slug: "s2", CreatedAt: 2000},
				}
				m.selected = rowCursor(0)
				_, y := findRow(t, m, 0)
				term := renderSettingsToEmulator(t, m.View(), m.width, m.height)
				cell := func(col, row int) string {
					if c := term.CellAt(col, row); c != nil {
						return c.Content
					}
					return ""
				}
				if got := cell(gutterTestX, y); got != ">" {
					t.Fatalf("rendered gutter cell (%d,%d) = %q, want the > marker", gutterTestX, y, got)
				}
				for x := 0; x < 8; x++ {
					want := x == 2 || x == 3
					if hit := m.hitTest(x, y); hit.gutter != want {
						t.Fatalf("hitTest(%d,%d).gutter = %v, want %v (%+v)", x, y, hit.gutter, want, hit)
					}
				}
				// The same cells on the unselected row are the gutter too.
				_, y1 := findRow(t, m, 1)
				if hit := m.hitTest(3, y1); !hit.gutter || hit.sessionIndex != 1 {
					t.Fatalf("hitTest(3,%d) = %+v, want the second row's gutter", y1, hit)
				}
			})
		}
	}
}
