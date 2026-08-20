package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestSessionsLoadedRendersInAttentionOrder is task 038's own proof that
// sortSessionsByAttention (task 023) is not merely defined but actually
// wired into Model's render order: a mixed-status load must come out of
// Update in exactly the requirement-28/29 order, matching SPEC §11's own
// illustration where a group's position follows its most urgent member.
func TestSessionsLoadedRendersInAttentionOrder(t *testing.T) {
	sessions := []store.Session{
		{ID: "running-one", Status: "running", StatusAt: 100},
		{ID: "idle-one", Status: "idle", StatusAt: 100},
		{ID: "waiting-one", Status: "waiting", StatusAt: 100},
		{ID: "error-one", Status: "error", StatusAt: 100},
	}
	var model Model
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	model = updated.(Model)

	var got []string
	for _, s := range model.sessions {
		got = append(got, s.ID)
	}
	want := []string{"waiting-one", "error-one", "running-one", "idle-one"}
	if len(got) != len(want) {
		t.Fatalf("rendered order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rendered order = %v, want %v", got, want)
		}
	}
}

// TestSessionsLoadedTracksSelectionByIDAcrossResort proves the reload path
// keeps the same session selected when a resort (e.g. a status change
// observed by the next reconcile tick) moves it to a different index,
// rather than leaving m.selected pointing at whatever session now sits at
// the old index.
func TestSessionsLoadedTracksSelectionByIDAcrossResort(t *testing.T) {
	first := []store.Session{
		{ID: "a", Status: "running", StatusAt: 100},
		{ID: "b", Status: "waiting", StatusAt: 100},
	}
	var model Model
	updated, _ := model.Update(sessionsLoaded{sessions: first})
	model = updated.(Model)
	// After the first load: [b (waiting), a (running)]. Select "a" (index 1).
	if model.sessions[1].ID != "a" {
		t.Fatalf("setup: sessions[1] = %q, want %q", model.sessions[1].ID, "a")
	}
	model.selected = 1

	// A reload where "a" is now waiting (and so sorts to the front) and a
	// brand new session "c" is running must keep "a" selected, even though
	// its index moved from 1 to 0.
	second := []store.Session{
		{ID: "a", Status: "waiting", StatusAt: 200},
		{ID: "b", Status: "waiting", StatusAt: 100},
		{ID: "c", Status: "running", StatusAt: 300},
	}
	updated, _ = model.Update(sessionsLoaded{sessions: second})
	model = updated.(Model)
	if got := model.sessions[model.selected].ID; got != "a" {
		t.Fatalf("selected session after resort = %q, want %q (selected index now %d)", got, "a", model.selected)
	}
}

// TestSessionsLoadedClampsWhenSelectedSessionVanishes proves a reload that
// drops the previously selected session (e.g. it was deleted) still leaves
// m.selected within bounds rather than panicking on the next index into
// m.sessions.
func TestSessionsLoadedClampsWhenSelectedSessionVanishes(t *testing.T) {
	first := []store.Session{
		{ID: "a", Status: "running", StatusAt: 100},
		{ID: "b", Status: "waiting", StatusAt: 100},
	}
	var model Model
	updated, _ := model.Update(sessionsLoaded{sessions: first})
	model = updated.(Model)
	// [b, a]; select "a" at index 1.
	model.selected = 1

	second := []store.Session{
		{ID: "b", Status: "waiting", StatusAt: 100},
	}
	updated, _ = model.Update(sessionsLoaded{sessions: second})
	model = updated.(Model)
	if model.selected < 0 || model.selected >= len(model.sessions) {
		t.Fatalf("selected = %d out of bounds for %d sessions", model.selected, len(model.sessions))
	}
	// Access must not panic.
	_ = model.sessions[model.selected]
}
