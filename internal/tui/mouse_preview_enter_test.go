package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// previewPressXY finds an absolute (x, y) that hitTest resolves to
// hitPanelPreview for m's current side-by-side layout, mirroring
// mouse_test.go's own findRow/findHeader rather than hand-deriving the
// seam offset a second time.
func previewPressXY(t *testing.T, m Model) (x, y int) {
	t.Helper()
	layout := m.computeLayout()
	x, y = layout.Sidebar.Width+3, 5
	if hit := m.hitTest(x, y); hit.panel != hitPanelPreview {
		t.Fatalf("test setup: (%d,%d) hit-tests to %+v, want hitPanelPreview", x, y, hit)
	}
	return x, y
}

// TestPreviewPressEntersInteractiveOnCurrentSelectionAndRecordsAttachment
// is R144/GH #37's core list-mode claim: a left press over the passive
// preview runs enterInteractive on the CURRENT selection -- never moving
// it -- and records the same durable attachment `\u21b5` records
// (store.RecordAttachment answering a waiting row, proxied here through
// m.prepareAttach exactly like TestEnterInteractiveRecordsAttachment
// proves for the key itself). Proven against a real tmux server on a
// private socket because enterInteractive's success path cannot be
// reached any other way.
func TestPreviewPressEntersInteractiveOnCurrentSelectionAndRecordsAttachment(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "sess-prev-1", Name: "preventer", Slug: "preventer", Status: "waiting"},
		{ID: "sess-prev-2", Name: "other", Slug: "preventerother", Status: "waiting"},
	})
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("preventer")
	newQuietSelectionPane(t, socket, "deck_preventer", 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.selected = rowCursor(0)
	var recorded []string
	m.prepareAttach = func(_ context.Context, id string) error {
		recorded = append(recorded, id)
		return nil
	}

	x, y := previewPressXY(t, m)
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("a preview press returned a non-nil cmd, want nil (enterInteractive's own success path returns nil)")
	}
	if !got.interactive {
		t.Fatalf("a left press over the passive preview did not enter interactive mode")
	}
	if got.selected != rowCursor(0) {
		t.Fatalf("a preview press changed the selection to %v, want it to stay on the current selection rowCursor(0)", got.selected)
	}
	if len(recorded) != 1 || recorded[0] != "sess-prev-1" {
		t.Fatalf("prepareAttach calls = %v, want exactly one with the current selection's ID %q -- a preview press must record the same attachment \u21b5 records", recorded, "sess-prev-1")
	}
	got.exitInteractive()
}

// TestPreviewPressRefusesOnStoppedAndAttachedElsewhereSessions is R144's
// refusal half: a preview press over a session enterInteractive itself
// would refuse draws the exact same R143 banner state (entryRefusal)
// \u21b5 leaves behind, never a silent no-op and never a second, divergent
// refusal ladder.
func TestPreviewPressRefusesOnStoppedAndAttachedElsewhereSessions(t *testing.T) {
	t.Run("stopped", func(t *testing.T) {
		m := mouseTestModel([]store.Session{
			{ID: "sess-prev-stopped", Name: "prevstopped", Slug: "prevstopped", Status: "stopped"},
		})
		m.width, m.height = 100, 30
		m.tmuxClient = tmux.Client{Socket: "deck-tui-prev-no-server"}
		m.selected = rowCursor(0)

		x, y := previewPressXY(t, m)
		updated, _ := m.Update(press(x, y))
		got := updated.(Model)
		if got.interactive {
			t.Fatalf("a preview press entered interactive mode against a stopped session")
		}
		if !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalStopped {
			t.Fatalf("entryRefusal = %+v, want the active stopped-session refusal (R143's banner)", got.entryRefusal)
		}
	})

	t.Run("attached elsewhere", func(t *testing.T) {
		m := mouseTestModel([]store.Session{
			{ID: "sess-prev-attached", Name: "prevattached", Slug: "prevattached", Status: "waiting"},
		})
		m.width, m.height = 100, 30
		if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
		}

		socket := selectionTestSocket("prevattached")
		newQuietSelectionPane(t, socket, "deck_prevattached", 80, 24)
		client := tmux.Client{Socket: socket}
		m.tmuxClient = client
		m.selected = rowCursor(0)

		windowTarget, err := tmux.SessionName("prevattached")
		if err != nil {
			t.Fatalf("SessionName: %v", err)
		}
		attachForceEnterPTY(t, socket, windowTarget)
		waitForSessionAttachedCountForce(t, client, windowTarget, 1)

		x, y := previewPressXY(t, m)
		updated, _ := m.Update(press(x, y))
		got := updated.(Model)
		if got.interactive {
			t.Fatalf("a preview press entered interactive mode while a real client was attached elsewhere")
		}
		if !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalAttachedElsewhere {
			t.Fatalf("entryRefusal = %+v, want the active attached-elsewhere refusal (R143's banner)", got.entryRefusal)
		}
	})
}

// TestPreviewPressIsInertOnHeaderCursorStartsNoDragToCopyAndWheelStaysNoOp
// covers R144's remaining list-mode guarantees together: with the cursor
// resting on a group header (no selected session at all, so
// hasSelectedSession is false and enterInteractiveBody's own guard already
// refuses it) a preview press does nothing -- no interactive entry, no
// entryRefusal, no selection change, and no cmd -- and it never begins
// task 216's drag-to-copy gesture (list mode's own handleMousePress never
// calls beginInteractiveSelection at all; that path only exists inside
// Update's m.interactive branch). A wheel notch over the same passive
// preview stays the no-op TestWheelScrollsSidebarWithoutChangingSelectionOrFocus
// already pins, re-asserted here against the header-cursor model so this
// task's own probe exercises it under the exact setup R144 names.
func TestPreviewPressIsInertOnHeaderCursorStartsNoDragToCopyAndWheelStaysNoOp(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "sess-prev-header", Name: "prevheader", Slug: "prevheader", Status: "waiting"},
	})
	m.width, m.height = 100, 30
	m.selected = headerCursor(0)

	x, y := previewPressXY(t, m)
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("a preview press over a header cursor returned a non-nil cmd, want nil")
	}
	if got.interactive {
		t.Fatalf("a preview press over a header cursor entered interactive mode")
	}
	if got.entryRefusal.active {
		t.Fatalf("a preview press over a header cursor armed an entry refusal: %+v", got.entryRefusal)
	}
	if got.selected != headerCursor(0) {
		t.Fatalf("a preview press over a header cursor changed the selection to %v", got.selected)
	}
	if got.interactiveSelecting {
		t.Fatalf("a preview press over a header cursor started drag-to-copy selection")
	}

	updated, cmd = got.Update(wheelDown(x, y))
	afterWheel := updated.(Model)
	if cmd != nil {
		t.Fatalf("a wheel notch over the passive preview returned a non-nil cmd, want nil")
	}
	if afterWheel.sidebarScroll != got.sidebarScroll {
		t.Fatalf("a wheel notch over the passive preview changed sidebarScroll: %d -> %d", got.sidebarScroll, afterWheel.sidebarScroll)
	}
	if afterWheel.selected != headerCursor(0) {
		t.Fatalf("a wheel notch over the passive preview changed the selection to %v", afterWheel.selected)
	}
}
