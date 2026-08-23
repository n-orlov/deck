package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode is requirement 30:
// the undo toast (undoNoteLines) renders outside the panel rectangles, the
// exact hazard requirement 37 already fixed for the startup banner,
// attachError and resumeNote -- a message drawn outside the panels must be
// counted in computeLayout's reserved rows or the frame grows past its
// budget and the golden 80x24 frame shears. Task 102 already added
// undoNoteLines to the reserved-row expression (tui.go's computeLayout);
// this pins that guarantee at the 80x24 minimum in every layout mode
// (auto, side-by-side, stacked, collapsed), asserting both the row count
// and every rendered line's visible column width stay inside the frame
// with the toast actually on screen.
func TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode(t *testing.T) {
	modes := []string{LayoutAuto, LayoutSideBySide, LayoutStacked, LayoutCollapsed}
	width, height := 80, 24
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			model := New(nil, config.Settings{}, "")
			model.sessions = []store.Session{{ID: "killed-1", Name: "killed-session", Agent: "shell", Status: "stopped"}}
			model.width, model.height = width, height
			model.layoutMode = mode
			// Put the undo toast on screen exactly as x does: name the
			// killed session so undoNoteLines returns non-nil.
			model.undoSessionID = "killed-1"
			model.undoSessionName = "killed-session"

			if model.undoNoteLines(width) == nil {
				t.Fatalf("test setup: undoNoteLines returned nil with undoSessionID set")
			}

			view := model.View()
			lines := strings.Split(view, "\n")
			if len(lines) > height {
				t.Fatalf("view has %d lines with the undo toast on screen, exceeding height %d:\n%s", len(lines), height, view)
			}
			// The footer's own key legend (always the last line) already
			// overflows 80 columns with no toast involved at all --
			// TestBelowMinimumFrameStaysWithinBudget's comment documents
			// this as "a separate, pre-existing defect out of this task's
			// scope" and scopes its own width assertion the same way.
			// This test stays scoped to the width budget the undo toast
			// is actually responsible for: every line above the footer.
			for i, line := range lines[:len(lines)-1] {
				if w := stringWidth(line); w > width {
					t.Fatalf("line %d has visible width %d with the undo toast on screen, exceeding width %d: %q", i, w, width, line)
				}
			}
			if !strings.Contains(view, "press u to undo") {
				t.Fatalf("view is missing the undo toast text with undoSessionID set:\n%s", view)
			}
		})
	}
}

// TestUndoToastBudgetHoldsAcrossSizes is the same guarantee as
// TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode but sweeps a range
// of terminal sizes (mirroring TestThemeFallbackNoticeStaysWithinFrameBudget
// and TestBelowMinimumFrameStaysWithinBudget's own convention), so the
// undo-toast reservation is proven at more than just the exact minimum.
func TestUndoToastBudgetHoldsAcrossSizes(t *testing.T) {
	sizes := [][2]int{{80, 24}, {70, 24}, {60, 20}, {100, 30}, {120, 24}}
	for _, size := range sizes {
		width, height := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
			model := New(nil, config.Settings{}, "")
			model.sessions = []store.Session{{ID: "killed-1", Name: "killed-session", Agent: "shell", Status: "stopped"}}
			model.width, model.height = width, height
			model.undoSessionID = "killed-1"
			model.undoSessionName = "killed-session"

			view := model.View()
			lines := strings.Split(view, "\n")
			if len(lines) > height {
				t.Fatalf("view has %d lines with the undo toast on screen, exceeding height %d:\n%s", len(lines), height, view)
			}
		})
	}
}
