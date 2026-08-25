package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestPendingSelectSessionIDSelectsNewRowNotIndexZero proves requirement
// 52's core promise: a session created via `n` is selected the moment it
// actually appears in a load, regardless of where the attention sort
// places it -- never assumed to land at index 0. The fixture pins the new
// session to "idle" (attentionRankIdle) behind an already-"waiting" row
// (attentionRankWaiting) precisely so a naive "select index 0" stand-in
// would fail this test.
func TestPendingSelectSessionIDSelectsNewRowNotIndexZero(t *testing.T) {
	var model Model
	model.pendingSelectSessionID = "brand-new"
	sessions := []store.Session{
		{ID: "already-here", Status: "waiting", StatusAt: 100},
		{ID: "brand-new", Status: "idle", StatusAt: 200},
	}
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)

	if model.sessions[0].ID != "already-here" || model.sessions[1].ID != "brand-new" {
		t.Fatalf("setup: rendered order = %v, want [already-here brand-new] (waiting outranks idle)", sessionIDs(model.sessions))
	}
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1 (brand-new's actual row, not index 0)", model.selected)
	}
	if model.sessions[model.selected].ID != "brand-new" {
		t.Fatalf("selected session = %q, want %q", model.sessions[model.selected].ID, "brand-new")
	}
	if model.pendingSelectSessionID != "" {
		t.Fatalf("pendingSelectSessionID = %q, want cleared once consumed", model.pendingSelectSessionID)
	}
}

// TestPendingSelectSessionIDIsOneShot proves the intent fires exactly
// once: once a load has consumed it and moved the selection, a LATER
// sessionsLoaded (e.g. the next reconcile tick) must not re-steal a
// selection the user has since moved elsewhere.
func TestPendingSelectSessionIDIsOneShot(t *testing.T) {
	var model Model
	model.pendingSelectSessionID = "brand-new"
	sessions := []store.Session{
		{ID: "already-here", Status: "waiting", StatusAt: 100},
		{ID: "brand-new", Status: "idle", StatusAt: 200},
	}
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)
	if model.selected != 1 {
		t.Fatalf("setup: selected = %d, want 1", model.selected)
	}

	// User moves the selection away from the just-created row.
	model.selected = 0

	// A later load (e.g. a reconcile tick) with the same sessions must
	// leave the user's own move alone.
	updated, _ = model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)
	if model.selected != 0 {
		t.Fatalf("selected = %d, want 0 (one-shot intent must not re-fire)", model.selected)
	}
	if model.sessions[model.selected].ID != "already-here" {
		t.Fatalf("selected session = %q, want %q", model.sessions[model.selected].ID, "already-here")
	}
}

// TestPendingSelectSessionIDNeverAppearing proves a pending intent whose
// id never shows up in any load (e.g. the created session vanished before
// the very first load that would have contained it) leaves the current
// selection exactly as the ordinary preserved-selection logic would have
// left it anyway, and never panics.
func TestPendingSelectSessionIDNeverAppearing(t *testing.T) {
	var model Model
	model.pendingSelectSessionID = "never-shows-up"
	sessions := []store.Session{
		{ID: "already-here", Status: "waiting", StatusAt: 100},
		{ID: "another-one", Status: "idle", StatusAt: 200},
	}
	model.selected = 1 // already-here.. picks whatever it resolves to below
	// Establish a baseline selection the ordinary preserved-selection path
	// would keep: select "another-one" first via a normal load.
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)
	model.pendingSelectSessionID = "never-shows-up"
	beforeSelectedID := model.sessions[model.selected].ID

	updated, _ = model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)

	if got := model.sessions[model.selected].ID; got != beforeSelectedID {
		t.Fatalf("selected session = %q, want unchanged %q", got, beforeSelectedID)
	}
}

// TestPendingSelectSessionIDRespectsActiveFilter proves requirement 52's
// filter guard (behaviour d): a filter query in force that excludes the
// freshly created session must leave BOTH the selection and the filter
// itself untouched -- the new row is in the store (msg.sessions /
// baseSessions) but not in the filtered view, so there is nothing visible
// to jump the selection to.
func TestPendingSelectSessionIDRespectsActiveFilter(t *testing.T) {
	var model Model
	// Seed a filtered baseline: "kept" matches the filter, "brand-new"
	// (created after the filter was already in force) does not.
	model.filterQuery = "kept"
	seed := []store.Session{
		{ID: "kept", Name: "kept-session", Status: "idle", StatusAt: 100},
	}
	updated, _ := model.Update(sessionsLoaded{sessions: seed})
	model = updated.(Model)
	if len(model.sessions) != 1 || model.sessions[0].ID != "kept" {
		t.Fatalf("setup: filtered sessions = %v, want just [kept]", sessionIDs(model.sessions))
	}
	model.selected = 0
	model.pendingSelectSessionID = "brand-new"

	withNew := []store.Session{
		{ID: "kept", Name: "kept-session", Status: "idle", StatusAt: 100},
		{ID: "brand-new", Name: "excluded-session", Status: "waiting", StatusAt: 200},
	}
	updated, _ = model.Update(sessionsLoaded{sessions: withNew})
	model = updated.(Model)

	if model.filterQuery != "kept" {
		t.Fatalf("filterQuery = %q, want unchanged %q", model.filterQuery, "kept")
	}
	if len(model.sessions) != 1 || model.sessions[0].ID != "kept" {
		t.Fatalf("filtered sessions = %v, want the filter to still exclude brand-new", sessionIDs(model.sessions))
	}
	if model.selected != 0 || model.sessions[model.selected].ID != "kept" {
		t.Fatalf("selection moved despite the new session being filtered out: selected=%d session=%v", model.selected, sessionIDs(model.sessions))
	}
}

// TestPendingSelectSessionIDScrollsIntoView proves the new row is not just
// selected but actually brought on screen: with enough sessions that the
// default 80x24 frame cannot show them all, a new session sorted well
// past the visible window must move m.sidebarScroll so its row is
// present in sidebarVisibleEntries, not merely correct in m.selected.
func TestPendingSelectSessionIDScrollsIntoView(t *testing.T) {
	var model Model
	var sessions []store.Session
	// 30 already-idle sessions -- far more than an 80x24 frame's sidebar
	// can show two-line rows for -- all ranking ahead of the new "waiting"
	// session would... no: pin the new session to sort dead last by using
	// a *stopped* status (least urgent) with a late StatusAt, so it lands
	// at the bottom of a long list that starts scrolled at the top.
	for i := 0; i < 30; i++ {
		sessions = append(sessions, store.Session{
			ID:       fmt.Sprintf("old-%02d", i),
			Name:     fmt.Sprintf("old-%02d", i),
			Status:   "idle",
			StatusAt: int64(i),
		})
	}
	model.pendingSelectSessionID = "brand-new"
	sessions = append(sessions, store.Session{ID: "brand-new", Name: "brand-new", Status: "stopped", StatusAt: 1000})

	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)

	idx := indexOfSessionID(model.sessions, "brand-new")
	if idx < 0 {
		t.Fatalf("brand-new not found in rendered sessions")
	}
	if model.selected != idx {
		t.Fatalf("selected = %d, want %d (brand-new's row)", model.selected, idx)
	}

	layout := model.computeLayout()
	contentWidth := max(layout.Sidebar.Width-2, 0)
	contentHeight := layout.Sidebar.Height - 2
	visible := model.sidebarVisibleEntries(contentWidth, contentHeight)
	found := false
	for _, e := range visible {
		if e.kind == sidebarLineRow && e.sessionIndex == idx {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("brand-new's row (session index %d) not within the visible window after scrollSessionIntoView; sidebarScroll=%d", idx, model.sidebarScroll)
	}
}

func sessionIDs(sessions []store.Session) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID
	}
	return out
}
