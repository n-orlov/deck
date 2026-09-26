package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// cure-01-02 (R143/SPEC §11.9): "clears ... on Esc -- which dismisses the
// banner BEFORE any other layer Esc clears". These regression tests pin the
// ordering through the real Model.Update dispatch, not the helper directly,
// for every reachable refusal-plus-layer combination the review found
// broken at ee7f5a5d3 (artifacts/review/reviewer_refusal_test.go,
// TestReviewRefusalEscPrecedesPendingDelete and
// TestReviewRefusalEscPrecedesHelp both failed there because the esc-clears-
// refusal check sat AFTER pendingDelete's own intercept and every dialog's
// own updater in the dispatch chain, so it was never reached while any of
// those layers were open).

func newRefusalEscFixture() Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "a", Name: "alpha", Slug: "alpha", Status: "stopped"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("a", entryRefusalStopped, "stopped")
	return m
}

// TestEntryRefusalEscPrecedesPendingDelete: refusal + an armed pendingDelete
// (from a lone `d`) + a non-empty mark set. The first Esc must clear only
// the refusal, leaving pendingDelete armed and marks untouched; the second
// Esc then falls through to pendingDelete's own existing behavior (any key
// other than a second `d` disarms it without acting).
func TestEntryRefusalEscPrecedesPendingDelete(t *testing.T) {
	m := newRefusalEscFixture()
	m.marked = map[string]bool{"a": true}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	if !m.pendingDelete {
		t.Fatalf("test setup: lone d did not arm pendingDelete")
	}
	if !m.entryRefusal.active {
		t.Fatalf("test setup: refusal is not active before Esc -- fixture is vacuous")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("first Esc: refusal still active, want dismissed before pendingDelete")
	}
	if !m.pendingDelete {
		t.Fatalf("first Esc: pendingDelete = false, want it untouched (still armed) while the refusal was the thing dismissed")
	}
	if !m.marked["a"] {
		t.Fatalf("first Esc: marks were cleared, want them untouched -- exactly one layer clears per press")
	}

	// Second Esc: refusal is gone, so this press reaches pendingDelete's own
	// existing behavior -- any key other than a second `d` disarms it
	// without performing the destructive action.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.pendingDelete {
		t.Fatalf("second Esc: pendingDelete still armed, want disarmed by pendingDelete's own existing Esc behavior")
	}
	if m.deleteConfirming {
		t.Fatalf("second Esc: deleteConfirming opened, want the delete swallowed like any non-`d` key after a lone `d`")
	}
	if !m.marked["a"] {
		t.Fatalf("second Esc: marks were cleared, want them untouched -- pendingDelete's own Esc behavior swallows the key, it does not clear marks")
	}
}

// TestEntryRefusalEscPrecedesHelp: refusal + help open. The first Esc must
// dismiss only the refusal, leaving help open; the second Esc then falls
// through to updateHelpView's own existing behavior and closes help.
func TestEntryRefusalEscPrecedesHelp(t *testing.T) {
	m := newRefusalEscFixture()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if !m.help {
		t.Fatalf("test setup: ? did not open help")
	}
	if !m.entryRefusal.active {
		t.Fatalf("test setup: refusal is not active before Esc -- fixture is vacuous")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("first Esc: refusal still active, want dismissed before help")
	}
	if !m.help {
		t.Fatalf("first Esc: help = false, want it untouched (still open) while the refusal was the thing dismissed")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.help {
		t.Fatalf("second Esc: help still open, want closed by updateHelpView's own existing Esc behavior")
	}
}

// TestEntryRefusalEscPrecedesMarkClear: refusal + a non-empty mark set, with
// no other Esc-cleared layer open. The first Esc dismisses only the
// refusal; the second Esc then falls through to the plain top-level Esc
// case and clears the marks (task 112's existing behavior).
func TestEntryRefusalEscPrecedesMarkClear(t *testing.T) {
	m := newRefusalEscFixture()
	m.marked = map[string]bool{"a": true}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("first Esc: refusal still active, want dismissed before the mark clear")
	}
	if !m.marked["a"] {
		t.Fatalf("first Esc: marks were cleared, want them untouched -- exactly one layer clears per press")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.marked["a"] {
		t.Fatalf("second Esc: marks still set, want cleared by the plain top-level Esc's existing behavior")
	}
}

// TestEntryRefusalEscPrecedesFiltering: refusal + an open filter text field.
// The first Esc dismisses only the refusal, leaving filtering open and the
// query held; the second Esc then falls through to updateFilter's own
// existing behavior.
func TestEntryRefusalEscPrecedesFiltering(t *testing.T) {
	m := newRefusalEscFixture()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	if !m.filtering {
		t.Fatalf("test setup: / did not open filtering")
	}
	if !m.entryRefusal.active {
		t.Fatalf("test setup: refusal is not active before Esc -- fixture is vacuous")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("first Esc: refusal still active, want dismissed before filtering")
	}
	if !m.filtering {
		t.Fatalf("first Esc: filtering = false, want it untouched (still open) while the refusal was the thing dismissed")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.filtering {
		t.Fatalf("second Esc: filtering still open, want closed by updateFilter's own existing Esc behavior")
	}
}
