package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSortSessionsByAttentionStablePrefersPreviousOrderOverIDCoinFlip is
// I-1/task 005's root-cause finding, isolated at the pure-function level:
// two sessions tie on BOTH rank and StatusAt (the exact "promoted in the
// same reconcile pass" collision docs/reports/phase3d-i1-rootcause.md
// documents) and their IDs compare the "wrong" way (the one that was
// already first, by an established previous frame, has the lexically
// LARGER ID). sortSessionsByAttention itself would flip them purely on
// that coin flip; sortSessionsByAttentionStable must not, because both
// already appeared together in `previous` in the other order.
func TestSortSessionsByAttentionStablePrefersPreviousOrderOverIDCoinFlip(t *testing.T) {
	previous := []store.Session{
		{ID: "zzz-created-first", Name: "bk-one", Status: "starting", StatusAt: 100},
		{ID: "aaa-created-second", Name: "bk-two", Status: "starting", StatusAt: 200},
	}
	// Both promoted to running in the same reconcile pass: identical
	// StatusAt. "aaa-created-second" sorts first by plain ID order even
	// though it was created (and previously rendered) second.
	incoming := []store.Session{
		{ID: "zzz-created-first", Name: "bk-one", Status: "running", StatusAt: 500},
		{ID: "aaa-created-second", Name: "bk-two", Status: "running", StatusAt: 500},
	}

	plain := sortSessionsByAttention(incoming)
	if plain[0].Name != "bk-two" {
		t.Fatalf("sanity check failed: expected the plain ID tie-break to put bk-two first, got %v", idsOf(plain))
	}

	got := sortSessionsByAttentionStable(previous, incoming)
	if got[0].Name != "bk-one" || got[1].Name != "bk-two" {
		t.Fatalf("sortSessionsByAttentionStable = %v, want [bk-one bk-two] (previous relative order preserved)", idsOf(got))
	}
}

// TestSortSessionsByAttentionStableFallsBackToIDWithNoPreviousFrame proves
// the fallback: two sessions with no shared entry in `previous` (both new
// this load) still resolve exactly like sortSessionsByAttention's own ID
// order -- there is no earlier order to prefer, so this must not diverge
// from the pure function's documented, unit-tested behaviour
// (TestSortSessionsByAttentionTiesBrokenByID).
func TestSortSessionsByAttentionStableFallsBackToIDWithNoPreviousFrame(t *testing.T) {
	incoming := []store.Session{
		{ID: "zebra", Status: "waiting", StatusAt: 500},
		{ID: "alpha", Status: "waiting", StatusAt: 500},
	}
	got := sortSessionsByAttentionStable(nil, incoming)
	if got[0].ID != "alpha" || got[1].ID != "zebra" {
		t.Fatalf("got %v, want [alpha zebra] (ID fallback, no previous frame)", idsOf(got))
	}
}

// TestMarkedSetKMJSurvivesBothSessionsRacingToRunningTogether is the exact
// requirement-29 marked-set idiom (k, m, j) reproduced at the Model level:
// two sessions created moments apart, both still "starting" on the first
// load (establishing bk-one before bk-two, the actual creation order), then
// BOTH promoted to "running" in one single subsequent load with an
// IDENTICAL StatusAt -- exactly what a reconcile pass promoting two
// co-created shells in the same pass produces (see
// docs/reports/phase3d-i1-rootcause.md). bk-two's session ID is
// deliberately picked lexically smaller than bk-one's, so a plain
// ID-tie-break sort (sortSessionsByAttention) would swap them and this
// test is red without sortSessionsByAttentionStable wired into
// Model's sessionsLoaded handler: "k" (already topmost) stays a no-op,
// "m" marks the row that must stay bk-one, and "j" must land on bk-two --
// never leaving the selection stuck on the first marked row the way I-1's
// three requirement-29 scenarios did.
func TestMarkedSetKMJSurvivesBothSessionsRacingToRunningTogether(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{}, "", nil)

	// First load: both still starting, distinct StatusAt (real creation
	// times) -- bk-one before bk-two, exactly like the real fixture.
	got, _ := model.Update(sessionsLoaded{sessions: []store.Session{
		{ID: "zzz-bk-one", Name: "bk-one", Status: "starting", StatusAt: 100},
		{ID: "aaa-bk-two", Name: "bk-two", Status: "starting", StatusAt: 200},
	}})
	model = got.(Model)
	if model.sessions[0].Name != "bk-one" || model.sessions[1].Name != "bk-two" {
		t.Fatalf("first load order = %v, want [bk-one bk-two]", idsOf(model.sessions))
	}

	// "k" (up): already topmost, must stay a no-op.
	got, _ = model.Update(key("k"))
	model = got.(Model)
	if model.selected != 0 {
		t.Fatalf("k moved off the topmost row: selected=%d", model.selected)
	}

	// Second load: both promoted to running in the same reconcile pass --
	// identical StatusAt, and bk-two's ID sorts before bk-one's ID.
	got, _ = model.Update(sessionsLoaded{sessions: []store.Session{
		{ID: "zzz-bk-one", Name: "bk-one", Status: "running", StatusAt: 9000},
		{ID: "aaa-bk-two", Name: "bk-two", Status: "running", StatusAt: 9000},
	}})
	model = got.(Model)
	if model.sessions[model.selected].Name != "bk-one" {
		t.Fatalf("selection followed the wrong session after the tied promotion: now selected %q", model.sessions[model.selected].Name)
	}

	// "m": marks bk-one.
	got, _ = model.Update(key("m"))
	model = got.(Model)
	if !model.marked["zzz-bk-one"] {
		t.Fatalf("m did not mark bk-one: marked=%#v", model.marked)
	}

	// "j" (down): must reach bk-two -- the whole point of the idiom.
	got, _ = model.Update(key("j"))
	model = got.(Model)
	if model.sessions[model.selected].Name != "bk-two" {
		t.Fatalf("after k,m,j selected=%q, want bk-two (I-1's exact failure: stuck on the first marked row)", model.sessions[model.selected].Name)
	}
}
