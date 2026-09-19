package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestMarkTogglesBySessionIDAndSurvivesReorderAndRegroup proves task 112's
// "the mark set survives a re-sort and a re-group" directly: m.marked is
// keyed by session id, never by index or visual position, so reassigning
// m.sessions to a completely different order (exactly what a real
// loadSessions/attention re-sort does) and collapsing a workspace group
// (a re-group) must not move which sessions markedSessions() returns.
func TestMarkTogglesBySessionIDAndSurvivesReorderAndRegroup(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "running", CWD: "/tmp/work-a"},
		{ID: "s2", Name: "beta", Status: "running", CWD: "/tmp/work-b"},
		{ID: "s3", Name: "gamma", Status: "running", CWD: "/tmp/work-a"},
	}

	model.selected = 0
	got, _ := model.Update(key("m"))
	model = got.(Model)
	model.selected = 2
	got, _ = model.Update(key("m"))
	model = got.(Model)

	if !model.marked["s1"] || !model.marked["s3"] || model.marked["s2"] {
		t.Fatalf("marked = %#v, want exactly s1 and s3", model.marked)
	}

	// Toggling s1 again clears it -- m is a plain toggle, not "add only".
	model.selected = 0
	got, _ = model.Update(key("m"))
	model = got.(Model)
	if model.marked["s1"] {
		t.Fatal("a second m on the same row did not clear its mark")
	}
	model.selected = 0
	got, _ = model.Update(key("m"))
	model = got.(Model)
	if !model.marked["s1"] {
		t.Fatal("m did not re-mark s1")
	}

	// Simulate a re-sort (reorder m.sessions, exactly like a real
	// loadSessions attention re-sort would) AND a re-group (collapse a
	// workspace group) in the same step.
	model.sessions = []store.Session{model.sessions[2], model.sessions[1], model.sessions[0]}
	model.collapsedGroups = map[int64]bool{sessionGroupID(model.sessions[0]): true}

	names := map[string]bool{}
	for _, s := range model.markedSessions() {
		names[s.Name] = true
	}
	if !names["alpha"] || !names["gamma"] || names["beta"] {
		t.Fatalf("markedSessions after reorder+regroup = %#v, want alpha and gamma only", names)
	}
}

// TestEscClearsMarkSet proves "the marks clear on ... esc" at the plain
// top-level (no dialog open).
func TestEscClearsMarkSet(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Status: "running"}}
	model.selected = 0

	got, _ := model.Update(key("m"))
	model = got.(Model)
	if len(model.marked) != 1 {
		t.Fatalf("expected 1 marked session, got %#v", model.marked)
	}

	got, _ = model.Update(key("esc"))
	model = got.(Model)
	if len(model.marked) != 0 {
		t.Fatalf("esc did not clear the mark set: %#v", model.marked)
	}
}

// TestBulkKillActsOnMarkedSetSkipsAlreadyStoppedAndOneUndoRestoresTheBatch
// proves requirement 28's core: x with a non-empty mark set kills every
// marked NON-stopped session (never the already-stopped one, silently
// skipped rather than refusing the whole batch), clears the marks the
// instant x is pressed, and a single u afterward resumes every session x
// just killed -- ONE undo for the whole batch, not N.
func TestBulkKillActsOnMarkedSetSkipsAlreadyStoppedAndOneUndoRestoresTheBatch(t *testing.T) {
	killed := map[string]bool{}
	resumed := map[string]bool{}

	model := NewWithShellCreator(nil, config.Settings{Undo: time.Hour}, "", nil)
	model.kill = func(_ context.Context, s store.Session) error {
		killed[s.ID] = true
		return nil
	}
	model.resume = func(_ context.Context, id string) (store.Session, service.ResumeOutcome, error) {
		resumed[id] = true
		return store.Session{ID: id, Status: "running"}, service.ResumeStarted, nil
	}
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "running"},
		{ID: "s2", Name: "beta", Status: "running"},
		{ID: "s3", Name: "gamma", Status: "stopped"},
	}
	for _, idx := range []int{0, 1, 2} {
		model.selected = idx
		got, _ := model.Update(key("m"))
		model = got.(Model)
	}
	if len(model.marked) != 3 {
		t.Fatalf("expected all 3 marked before x, got %#v", model.marked)
	}

	got, cmd := model.Update(key("x"))
	model = got.(Model)
	if len(model.marked) != 0 {
		t.Fatalf("marks did not clear the instant bulk x was pressed: %#v", model.marked)
	}
	if cmd == nil {
		t.Fatal("bulk x returned no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)

	if !killed["s1"] || !killed["s2"] {
		t.Fatalf("bulk x did not kill both non-stopped marked sessions: %#v", killed)
	}
	if killed["s3"] {
		t.Fatal("bulk x killed an already-stopped marked session")
	}
	if len(model.batchUndoSessionIDs) != 2 {
		t.Fatalf("batchUndoSessionIDs = %#v, want exactly the 2 sessions actually killed", model.batchUndoSessionIDs)
	}

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if len(model.batchUndoSessionIDs) != 0 {
		t.Fatal("u did not clear the batch undo window")
	}
	if cmd == nil {
		t.Fatal("u returned no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	if !resumed["s1"] || !resumed["s2"] {
		t.Fatalf("the single batch u did not resume both killed sessions: %#v", resumed)
	}
}

// TestBulkDeleteOpensConfirmForMarkedSetAndOneUndoRestoresTheBatch proves
// requirement 28's dd half: dd on a non-empty mark set opens the same
// confirm dialog naming the batch, submitting deletes every marked
// session and clears the marks at that moment, and a single u restores
// every session dd just tombstoned.
func TestBulkDeleteOpensConfirmForMarkedSetAndOneUndoRestoresTheBatch(t *testing.T) {
	deleted := map[string]bool{}
	restored := map[string]bool{}

	model := NewWithShellCreator(nil, config.Settings{DeleteGrace: time.Hour}, "", nil)
	model.deleteSvc = func(_ context.Context, s store.Session) error {
		deleted[s.ID] = true
		return nil
	}
	model.restoreSvc = func(_ context.Context, id string) (store.Session, error) {
		restored[id] = true
		return store.Session{ID: id}, nil
	}
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "stopped"},
		{ID: "s2", Name: "beta", Status: "stopped"},
	}
	for _, idx := range []int{0, 1} {
		model.selected = idx
		got, _ := model.Update(key("m"))
		model = got.(Model)
	}

	got, _ := model.Update(key("d"))
	model = got.(Model)
	if !model.pendingDelete {
		t.Fatal("first d did not set the pending-delete indicator for the marked set")
	}
	pending := model.pendingDeleteLines(80)
	if len(pending) == 0 || !strings.Contains(pending[0], "2") {
		t.Fatalf("pendingDeleteLines for a marked set does not name the batch size: %#v", pending)
	}

	got, _ = model.Update(key("d"))
	model = got.(Model)
	if !model.deleteConfirming {
		t.Fatal("second d did not open the confirm dialog for the marked set")
	}
	if len(model.marked) != 2 {
		t.Fatal("marks cleared before the batch delete was actually submitted")
	}
	body := model.deleteConfirmBody()
	if !strings.Contains(body, "2 marked sessions") {
		t.Fatalf("bulk delete confirm does not name the batch size:\n%s", body)
	}
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "beta") {
		t.Fatalf("bulk delete confirm does not name every marked session:\n%s", body)
	}
	if strings.Contains(body, "Purge:") {
		t.Fatalf("bulk delete confirm unexpectedly offers purge:\n%s", body)
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if len(model.marked) != 0 {
		t.Fatal("marks did not clear the instant the bulk delete was submitted")
	}
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	if !deleted["s1"] || !deleted["s2"] {
		t.Fatalf("bulk delete did not delete both marked sessions: %#v", deleted)
	}
	if len(model.batchDeleteUndoSessionIDs) != 2 {
		t.Fatalf("batchDeleteUndoSessionIDs = %#v, want both deleted sessions", model.batchDeleteUndoSessionIDs)
	}

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if len(model.batchDeleteUndoSessionIDs) != 0 {
		t.Fatal("u did not clear the batch delete undo window")
	}
	if cmd == nil {
		t.Fatal("u returned no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	if !restored["s1"] || !restored["s2"] {
		t.Fatalf("the single batch u did not restore both deleted sessions: %#v", restored)
	}
}

// TestBulkDeleteInvokesDeleterOncePerMarkedSessionWithItsOwnRow is task
// 017's evidence that a bulk dd over a two-session mark set drives
// deleteSvc -- and therefore internal/service's per-session post_destroy
// hook chain (task 013) -- once for EACH marked session, each call
// carrying that session's own row, rather than collapsing to a single
// call. Unlike the map-keyed assertions above, invocations is an
// append-only SLICE recorded in call order: a bulk delete that collapsed
// to one call (even one that happened to touch both session IDs some
// other way) would leave len(invocations) == 1 here and fail the count
// check below, which a map of booleans could never catch.
func TestBulkDeleteInvokesDeleterOncePerMarkedSessionWithItsOwnRow(t *testing.T) {
	var invocations []store.Session

	model := NewWithShellCreator(nil, config.Settings{DeleteGrace: time.Hour}, "", nil)
	model.deleteSvc = func(_ context.Context, s store.Session) error {
		invocations = append(invocations, s)
		return nil
	}
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "stopped", CWD: "/tmp/work-a"},
		{ID: "s2", Name: "beta", Status: "stopped", CWD: "/tmp/work-b"},
	}
	for _, idx := range []int{0, 1} {
		model.selected = idx
		got, _ := model.Update(key("m"))
		model = got.(Model)
	}
	if len(model.marked) != 2 {
		t.Fatalf("expected both sessions marked before dd, got %#v", model.marked)
	}

	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	if !model.deleteConfirming {
		t.Fatal("second d did not open the bulk delete confirm dialog")
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	got, _ = model.Update(cmd())
	_ = got.(Model)

	// The core assertion: exactly one invocation per marked session, never
	// one collapsed call for the whole batch.
	if len(invocations) != 2 {
		t.Fatalf("deleteSvc invoked %d time(s) for a 2-session bulk dd, want exactly 2 (one per marked session): %#v", len(invocations), invocations)
	}

	byID := map[string]store.Session{}
	for _, s := range invocations {
		if _, dup := byID[s.ID]; dup {
			t.Fatalf("deleteSvc invoked more than once for session %q: %#v", s.ID, invocations)
		}
		byID[s.ID] = s
	}
	s1, ok := byID["s1"]
	if !ok || s1.Name != "alpha" || s1.CWD != "/tmp/work-a" {
		t.Fatalf("deleteSvc's s1 invocation did not carry s1's own row: %#v", s1)
	}
	s2, ok := byID["s2"]
	if !ok || s2.Name != "beta" || s2.CWD != "/tmp/work-b" {
		t.Fatalf("deleteSvc's s2 invocation did not carry s2's own row: %#v", s2)
	}
}
