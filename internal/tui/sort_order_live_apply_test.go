package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestSettingsSaveResortsLivePreservingSelectionByID proves task 306's own
// success criterion: a save that changes [ui] sort_order re-sorts the
// ALREADY-RUNNING model (never waiting for a restart, since ui.sort_order
// is ScopeGlobal as of this task) with the selected SESSION -- not the
// selected INDEX -- staying put, and its row still inside the sidebar's
// visible window even though the reorder moves it far enough that an
// index-preserving (or non-scroll-adjusting) implementation would either
// select the wrong session or leave the right one scrolled off screen.
//
// Fixture: 20 sessions, "created" order is the exact reverse of the
// default attention/insertion order used to seed baseSessions (ties
// falling back to insertion order), so the selected session's index
// changes from 0 to 19 -- both far enough to fail an index-preserving
// selection AND far enough that the default 80x24 frame's sidebar (which
// cannot show 20 two-line rows at once) would leave the row off screen
// without scrollSessionIntoView's help.
func TestSettingsSaveResortsLivePreservingSelectionByID(t *testing.T) {
	model, _ := settingsLiveApplyTestModel(t)

	var sessions []store.Session
	for i := 0; i < 20; i++ {
		sessions = append(sessions, store.Session{
			ID:        fmt.Sprintf("s%02d", i),
			Name:      fmt.Sprintf("session-%02d", i),
			Agent:     "shell",
			Status:    "idle",
			CreatedAt: int64(i),
			StatusAt:  int64(i),
		})
	}
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)

	targetID := "s00"
	idx := indexOfSessionID(model.sessions, targetID)
	if idx != 0 {
		t.Fatalf("setup: %s is at index %d, want 0 (attention order ties fall back to insertion order)", targetID, idx)
	}
	model.selected = idx

	// Stage a switch to "created" order: CreatedAt descending puts s19
	// first and s00 (this test's selection) dead last, index 19 -- as far
	// from index 0 as this fixture can push it.
	model.settingsEdits = settingsEditsFromSettings(model.settings)
	model.settingsEdits.SortOrder = SortOrderCreated

	m := &model
	m.settingsSave()
	model = *m

	if model.settings.SortOrder != SortOrderCreated {
		t.Fatalf("m.settings.SortOrder = %q after save, want %q -- settingsApplyLiveFields did not copy the edit live", model.settings.SortOrder, SortOrderCreated)
	}

	newIdx := indexOfSessionID(model.sessions, targetID)
	if newIdx < 0 {
		t.Fatalf("%s missing from m.sessions after live resort", targetID)
	}
	if newIdx != 19 {
		t.Fatalf("setup: %s landed at index %d after resort, want 19 (fixture no longer non-vacuous)", targetID, newIdx)
	}
	if model.selected != newIdx {
		t.Fatalf("selected index = %d after live resort, want %d (the row %s now occupies) -- selection followed the OLD index, not the session id", model.selected, newIdx, targetID)
	}
	if model.sessions[model.selected].ID != targetID {
		t.Fatalf("selected session = %q after live resort, want %q", model.sessions[model.selected].ID, targetID)
	}

	layout := model.computeLayout()
	contentWidth := max(layout.Sidebar.Width-2, 0)
	contentHeight := layout.Sidebar.Height - 2
	visible := model.sidebarVisibleEntries(contentWidth, contentHeight)
	found := false
	for _, e := range visible {
		if e.kind == sidebarLineRow && e.sessionIndex == model.selected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s's row (session index %d) is not within the sidebar's visible window after the live re-sort; sidebarScroll=%d", targetID, model.selected, model.sidebarScroll)
	}
}
