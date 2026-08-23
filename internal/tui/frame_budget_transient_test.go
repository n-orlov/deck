package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestPendingDeleteIndicatorStaysWithinFrameBudgetAtEveryLayoutMode is
// requirement 30's residual named by I-15: the pending-`d` indicator
// (pendingDeleteLines, task 105/112) is drawn outside the panel
// rectangles exactly like the undo toast
// (TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode's own hazard), so
// it must be counted in computeLayout's reserved rows too. computeLayout
// already adds len(m.pendingDeleteLines(width)) to its reservation (see
// tui.go); this pins that guarantee at the 80x24 minimum in every layout
// mode, covering both the single-row wording ("Delete %q?") and the
// marked-set wording ("Delete %d marked sessions?") task 112 added.
func TestPendingDeleteIndicatorStaysWithinFrameBudgetAtEveryLayoutMode(t *testing.T) {
	modes := []string{LayoutAuto, LayoutSideBySide, LayoutStacked, LayoutCollapsed}
	width, height := 80, 24
	cases := []struct {
		name   string
		marked bool
		want   string
	}{
		{name: "single-row", marked: false, want: "press d again to confirm"},
		{name: "marked-set", marked: true, want: "Delete 2 marked sessions"},
	}
	for _, tc := range cases {
		for _, mode := range modes {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				model := New(nil, config.Settings{}, "")
				model.sessions = []store.Session{
					{ID: "s1", Name: "pending-delete-session", Agent: "shell", Status: "stopped"},
					{ID: "s2", Name: "second-session", Agent: "shell", Status: "stopped"},
				}
				model.width, model.height = width, height
				model.layoutMode = mode
				model.pendingDelete = true
				if tc.marked {
					model.marked = map[string]bool{"s1": true, "s2": true}
				}

				if model.pendingDeleteLines(width) == nil {
					t.Fatalf("test setup: pendingDeleteLines returned nil with pendingDelete set")
				}

				view := model.View()
				lines := strings.Split(view, "\n")
				if len(lines) > height {
					t.Fatalf("view has %d lines with the pending-delete indicator on screen, exceeding height %d:\n%s", len(lines), height, view)
				}
				for i, line := range lines[:len(lines)-1] {
					if w := stringWidth(line); w > width {
						t.Fatalf("line %d has visible width %d with the pending-delete indicator on screen, exceeding width %d: %q", i, w, width, line)
					}
				}
				if !strings.Contains(view, tc.want) {
					t.Fatalf("view is missing %q with pendingDelete set:\n%s", tc.want, view)
				}
			})
		}
	}
}

// TestDeleteUndoNoteStaysWithinFrameBudgetAtEveryLayoutMode is requirement
// 30's residual for the dd undo toast (deleteUndoNoteLines, task 106) and
// its batch-undo sibling (task 112's batchDeleteUndoSessionIDs): both are
// already summed into computeLayout's reservation; this proves the
// guarantee at 80x24 in every layout mode, matching
// TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode's own shape for
// the sibling x/undo toast.
func TestDeleteUndoNoteStaysWithinFrameBudgetAtEveryLayoutMode(t *testing.T) {
	modes := []string{LayoutAuto, LayoutSideBySide, LayoutStacked, LayoutCollapsed}
	width, height := 80, 24
	cases := []struct {
		name  string
		batch bool
		want  string
	}{
		{name: "single-delete-undo", batch: false, want: "Deleted \u2014 press u to undo"},
		{name: "batch-delete-undo", batch: true, want: "Deleted 2 sessions \u2014 press u to undo"},
	}
	for _, tc := range cases {
		for _, mode := range modes {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				model := New(nil, config.Settings{}, "")
				model.sessions = []store.Session{{ID: "killed-1", Name: "killed-session", Agent: "shell", Status: "stopped"}}
				model.width, model.height = width, height
				model.layoutMode = mode
				if tc.batch {
					model.batchDeleteUndoSessionIDs = []string{"a", "b"}
				} else {
					model.deleteUndoSessionID = "killed-1"
					model.deleteUndoSessionName = "killed-session"
				}

				if model.deleteUndoNoteLines(width) == nil {
					t.Fatalf("test setup: deleteUndoNoteLines returned nil")
				}

				view := model.View()
				lines := strings.Split(view, "\n")
				if len(lines) > height {
					t.Fatalf("view has %d lines with the delete-undo note on screen, exceeding height %d:\n%s", len(lines), height, view)
				}
				for i, line := range lines[:len(lines)-1] {
					if w := stringWidth(line); w > width {
						t.Fatalf("line %d has visible width %d with the delete-undo note on screen, exceeding width %d: %q", i, w, width, line)
					}
				}
				if !strings.Contains(view, tc.want) {
					t.Fatalf("view is missing %q with the delete-undo note set:\n%s", tc.want, view)
				}
			})
		}
	}
}

// TestArchivePurgeAndRefusalMessagesStayWithinFrameBudgetAtEveryLayoutMode
// is requirement 30's remaining residual named by I-15: the archive
// failure/refusal wording ("Cannot archive: ...", "Archiving is
// unavailable"), the purge-partial-failure wording deleteConfirm's submit
// can leave behind ("Deleted, but purge failed: ..."), and the plain kill
// refusal ("Cannot kill: session is already stopped") all surface through
// the one shared m.attachError field, rendered by attachErrorLines and
// already summed into computeLayout's reservation (requirement 37).
// Unlike the undo toast and the pending-delete indicator, attachError is
// not tied to one specific action, so this test drives it with each of
// those four literal messages in turn, in every layout mode at 80x24, to
// pin the same guarantee for each distinct wording rather than trusting
// one generic attachError case to stand in for all of them.
func TestArchivePurgeAndRefusalMessagesStayWithinFrameBudgetAtEveryLayoutMode(t *testing.T) {
	modes := []string{LayoutAuto, LayoutSideBySide, LayoutStacked, LayoutCollapsed}
	width, height := 80, 24
	messages := []string{
		"Cannot archive: session is already archived",
		"Archiving is unavailable",
		"Deleted, but purge failed: no such file or directory",
		"Cannot kill: session is already stopped",
	}
	for _, msg := range messages {
		for _, mode := range modes {
			name := fmt.Sprintf("%s/%s", msg, mode)
			t.Run(name, func(t *testing.T) {
				model := New(nil, config.Settings{}, "")
				model.sessions = []store.Session{{ID: "s1", Name: "some-session", Agent: "shell", Status: "stopped"}}
				model.width, model.height = width, height
				model.layoutMode = mode
				model.attachError = msg

				if model.attachErrorLines(width) == nil {
					t.Fatalf("test setup: attachErrorLines returned nil with attachError set")
				}

				view := model.View()
				lines := strings.Split(view, "\n")
				if len(lines) > height {
					t.Fatalf("view has %d lines with %q on screen, exceeding height %d:\n%s", len(lines), msg, height, view)
				}
				for i, line := range lines[:len(lines)-1] {
					if w := stringWidth(line); w > width {
						t.Fatalf("line %d has visible width %d with %q on screen, exceeding width %d: %q", i, w, msg, width, line)
					}
				}
				if !strings.Contains(view, msg) {
					t.Fatalf("view is missing %q with attachError set:\n%s", msg, view)
				}
			})
		}
	}
}
