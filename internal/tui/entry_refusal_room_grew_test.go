package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// runOnePreviewTick dispatches a real previewTick message through Model's
// actual Update method -- cure-01-06's own "regression tests drive real
// resize and tick handling" clause, never a direct call into
// clearEntryRefusalIfRoomGrew. clearEntryRefusalIfRoomGrew itself runs
// INLINE inside the previewTick case (a pure re-measure of
// m.previewContentSize(), never a tmux round trip, unlike
// entryRefusalHolderCheck's own probe), so this one Update call already
// completes that half of the tick; any Cmd it also returns (the
// reschedule, capturePreview, previewFit, entryRefusalHolderCheck) is run
// too, exactly like the event loop would, so a real capture/fit failure
// against the fixture's tmux socket still surfaces rather than being
// silently skipped -- but never recursed into a second tick, so this
// helper models exactly one.
func runOnePreviewTick(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if cmd == nil {
		return m
	}
	runReply := func(msg tea.Msg) {
		if msg == nil {
			return
		}
		if _, isTick := msg.(previewTick); isTick {
			return
		}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			runReply(sub())
		}
		return m
	}
	runReply(msg)
	return m
}

// TestEntryRefusalRowFloorClearsAfterGrowthAndTick is cure-01-06's own
// recovery case for the row-floor kind (SPEC §11.9, R143/R148): entry is
// refused at 80x9 (below interactiveMinInnerRows, exactly like
// TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall's own
// fixture), a REAL resize grows the terminal well above the floor, and a
// REAL previewTick -- no keypress, no selection move -- finds the
// condition gone and clears the banner. The review probe this fixes
// (artifacts/review/reviewer_refusal_recovery_test.go,
// TestReviewRowFloorRefusalClearsAfterGrowthAndTick) failed against
// ee7f5a5d3 with exactly this scenario.
func TestEntryRefusalRowFloorClearsAfterGrowthAndTick(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-cure-01-06-a"})
	m.sessions = []store.Session{{ID: "s1", Name: "roomgrow", Agent: "shell", Status: "running", Slug: "roomgrow"}}
	m.selected = rowCursor(0)
	m.width, m.height = 80, 9

	next, cmd := m.enterInteractive()
	m = next.(Model)
	if cmd != nil || m.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("test setup: entry at 80x9 did not produce a row-floor refusal: cmd=%v refusal=%+v", cmd, m.entryRefusal)
	}

	// Real resize: the terminal grows comfortably above the floor. The
	// resize itself must not clear the refusal -- only the LATER tick
	// does, per this task's own criterion.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 240, Height: 30})
	m = next.(Model)
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test setup: resize to 240x30 is still below the floor: %d", height)
	}
	if !m.entryRefusal.active {
		t.Fatalf("the resize alone already cleared the refusal -- this test would no longer be exercising the tick")
	}

	m = runOnePreviewTick(t, m)

	if m.entryRefusal.active {
		t.Fatalf("row-floor refusal still active after the preview grew back above the floor and a later tick ran: %+v", m.entryRefusal)
	}
}

// TestEntryRefusalRowFloorStaysRefusedWhenStillBelowFloorOnTick is the
// still-refused twin: the same row-floor refusal, but no resize happens
// before the tick -- the condition (below the floor) remains true, so the
// tick must retain the refusal rather than clearing it unconditionally.
func TestEntryRefusalRowFloorStaysRefusedWhenStillBelowFloorOnTick(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-cure-01-06-b"})
	m.sessions = []store.Session{{ID: "s1", Name: "roomstillfloor", Agent: "shell", Status: "running", Slug: "roomstillfloor"}}
	m.selected = rowCursor(0)
	m.width, m.height = 80, 9

	next, cmd := m.enterInteractive()
	m = next.(Model)
	if cmd != nil || m.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("test setup: entry at 80x9 did not produce a row-floor refusal: cmd=%v refusal=%+v", cmd, m.entryRefusal)
	}

	m = runOnePreviewTick(t, m)

	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("row-floor refusal did not survive a tick while the preview is still below the floor: %+v", m.entryRefusal)
	}
}

// TestEntryRefusalShrankClearsAfterGrowthAndTick is the shrank kind's own
// recovery case: entry succeeds at a comfortable size, a REAL resize
// shrinks below the floor WHILE already interactive (tui.go's own
// WindowSizeMsg case, task 009/R143) exits interactive mode and sets an
// entryRefusalShrank refusal, a REAL resize grows back above the floor,
// and a REAL previewTick clears it -- again with no keypress and no
// selection move.
func TestEntryRefusalShrankClearsAfterGrowthAndTick(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-cure-01-06-c"})
	m.sessions = []store.Session{{ID: "s1", Name: "roomgrowshrank", Agent: "shell", Status: "running", Slug: "roomgrowshrank"}}
	m.selected = rowCursor(0)
	m.interactive = true
	m.width, m.height = 80, 24

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	m = next.(Model)
	if cmd != nil || m.interactive || m.entryRefusal.kind != entryRefusalShrank {
		t.Fatalf("test setup: shrink to 80x9 did not produce a shrank refusal: cmd=%v interactive=%v refusal=%+v", cmd, m.interactive, m.entryRefusal)
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 240, Height: 30})
	m = next.(Model)
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test setup: regrowth to 240x30 is still below the floor: %d", height)
	}
	if !m.entryRefusal.active {
		t.Fatalf("the resize alone already cleared the refusal -- this test would no longer be exercising the tick")
	}

	m = runOnePreviewTick(t, m)

	if m.entryRefusal.active {
		t.Fatalf("shrank refusal still active after the preview grew back above the floor and a later tick ran: %+v", m.entryRefusal)
	}
}

// TestEntryRefusalShrankStaysRefusedWhenStillBelowFloorOnTick is the
// shrank kind's still-refused twin: no regrowth happens before the tick.
func TestEntryRefusalShrankStaysRefusedWhenStillBelowFloorOnTick(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-cure-01-06-d"})
	m.sessions = []store.Session{{ID: "s1", Name: "roomstillshrank", Agent: "shell", Status: "running", Slug: "roomstillshrank"}}
	m.selected = rowCursor(0)
	m.interactive = true
	m.width, m.height = 80, 24

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	m = next.(Model)
	if cmd != nil || m.interactive || m.entryRefusal.kind != entryRefusalShrank {
		t.Fatalf("test setup: shrink to 80x9 did not produce a shrank refusal: cmd=%v interactive=%v refusal=%+v", cmd, m.interactive, m.entryRefusal)
	}

	m = runOnePreviewTick(t, m)

	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalShrank {
		t.Fatalf("shrank refusal did not survive a tick while the preview is still below the floor: %+v", m.entryRefusal)
	}
}

// TestEntryRefusalContentionRefusalUnaffectedByRoomGrewCheck proves
// clearEntryRefusalIfRoomGrew is scoped to exactly the two size-based
// kinds: an attached-elsewhere refusal at a COMFORTABLE size (well above
// the floor throughout, so the room-grew check's own condition is
// trivially already true) must NOT be cleared by a bare tick with no
// tmux client wired (entryRefusalHolderCheck itself is a no-op with an
// empty socket, per its own doc comment) -- proving the new size check
// added by this task does not accidentally widen to kinds SPEC never
// calls "the reason gone" for a room re-measure.
func TestEntryRefusalContentionRefusalUnaffectedByRoomGrewCheck(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "roomholder", Slug: "roomholder", Status: "running"}}
	m.selected = rowCursor(0)
	m.width, m.height = 240, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test setup: 240x30 is below the floor: %d", height)
	}
	m.setEntryRefusal("s1", entryRefusalAttachedElsewhere, "another client is attached to this session")

	m = runOnePreviewTick(t, m)

	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalAttachedElsewhere {
		t.Fatalf("contention refusal changed after a tick with the room already above the floor: %+v", m.entryRefusal)
	}
}
