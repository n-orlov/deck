package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestNeedsAttentionMatchesWaitingAndErrorOnly is requirement 31's own
// definition, pinned so the sort, attentionCount and `space` can never
// silently drift apart on what "needs me" means.
func TestNeedsAttentionMatchesWaitingAndErrorOnly(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"waiting", true},
		{"error", true},
		{"running", false},
		{"starting", false},
		{"idle", false},
		{"stopped", false},
		{"archived", false},
	}
	for _, c := range cases {
		if got := NeedsAttention(store.Session{Status: c.status}); got != c.want {
			t.Errorf("NeedsAttention(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

// TestNextAttentionSelectionWrapsAndSkipsInvisible covers requirement 32:
// `space` finds the next visible session needing attention, wrapping
// around the end of the list, and skips a session hidden by a collapsed
// workspace group even though that session's own status needs attention.
func TestNextAttentionSelectionWrapsAndSkipsInvisible(t *testing.T) {
	m := Model{sessions: []store.Session{
		{ID: "a", Workspace: "ws", Status: "running"},
		{ID: "b", Workspace: "ws", Status: "waiting"},
		{ID: "c", Workspace: "ws", Status: "running"},
		{ID: "d", Workspace: "hidden-ws", Status: "error"},
		{ID: "e", Workspace: "ws", Status: "idle"},
	}}

	// From "a" (index 0), the next session needing attention is "b".
	if got, ok := m.nextAttentionSelection(0); !ok || got != 1 {
		t.Fatalf("from 0: got (%d, %v), want (1, true)", got, ok)
	}

	// From "b" itself, the search wraps all the way around and lands back
	// on "b" (index 1) since it is the only visible session needing
	// attention once "d" is hidden below.
	m.setGroupCollapsed("hidden-ws", true)
	if got, ok := m.nextAttentionSelection(1); !ok || got != 1 {
		t.Fatalf("from 1 with hidden-ws collapsed: got (%d, %v), want (1, true)", got, ok)
	}

	// Expand the group again: "d" (index 3) is now reachable and, searching
	// forward from "c" (index 2), is the very next one.
	m.setGroupCollapsed("hidden-ws", false)
	if got, ok := m.nextAttentionSelection(2); !ok || got != 3 {
		t.Fatalf("from 2 with hidden-ws expanded: got (%d, %v), want (3, true)", got, ok)
	}
}

// TestNextAttentionSelectionNoopWhenNothingNeedsAttention proves the "does
// nothing observable" half of requirement 32.
func TestNextAttentionSelectionNoopWhenNothingNeedsAttention(t *testing.T) {
	m := Model{sessions: []store.Session{
		{ID: "a", Status: "running"},
		{ID: "b", Status: "idle"},
		{ID: "c", Status: "stopped"},
	}}
	if got, ok := m.nextAttentionSelection(1); ok || got != 1 {
		t.Fatalf("got (%d, %v), want (1, false)", got, ok)
	}
}

// TestSpaceMovesSelectionAndWrapsWithoutTouchingSessionStatus is the full
// wiring test for requirements 31/32: pressing `space` against a real
// model+store moves the selection to the next session needing attention,
// wraps around the list, and — critically, per §7's attach-clears-waiting
// caution named in the task — never changes any session's durable status
// row, proven byte-identical (via each row's JSON encoding, since
// store.Session holds slice/map fields plain struct equality would still
// have to marshal to compare meaningfully) before and after repeated
// presses.
func TestSpaceMovesSelectionAndWrapsWithoutTouchingSessionStatus(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	rows := []struct {
		id, status string
		statusAt   int64
	}{
		{"running-one", "running", 100},
		{"waiting-one", "waiting", 200},
		{"idle-one", "idle", 300},
		{"error-one", "error", 400},
	}
	for i, row := range rows {
		if _, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: row.id, Name: row.id, CWD: "/work/" + row.id, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: int64(i + 1), CreatedAt: int64(i + 1),
		}); err != nil {
			t.Fatal(err)
		}
		if row.status != "running" {
			if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
				SessionID: row.id, Status: row.status, Reason: "test", Source: "hook", At: row.statusAt,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	before := snapshotSessions(t, db, ctx, []string{"running-one", "waiting-one", "idle-one", "error-one"})

	settings := config.Settings{ASCII: true}
	model := New(db, settings, "")
	loaded := model.loadSessions().(sessionsLoaded)
	updated, _ := model.Update(loaded)
	model = updated.(Model)
	if len(model.sessions) != 4 {
		t.Fatalf("loaded %d sessions, want 4", len(model.sessions))
	}
	model.selected = 0 // running-one: not itself in need of attention

	updated, _ = model.Update(key(" "))
	model = updated.(Model)
	if got := model.sessions[model.selected].ID; got != "waiting-one" {
		t.Fatalf("after first space, selected = %q, want %q", got, "waiting-one")
	}

	// waiting-one -> next is error-one (idle-one is skipped, it does not
	// need attention).
	updated, _ = model.Update(key(" "))
	model = updated.(Model)
	if got := model.sessions[model.selected].ID; got != "error-one" {
		t.Fatalf("after second space, selected = %q, want %q", got, "error-one")
	}

	// error-one -> wraps all the way around back to waiting-one, since
	// running-one and idle-one still do not need attention.
	updated, _ = model.Update(key(" "))
	model = updated.(Model)
	if got := model.sessions[model.selected].ID; got != "waiting-one" {
		t.Fatalf("after third space (wrap), selected = %q, want %q", got, "waiting-one")
	}

	after := snapshotSessions(t, db, ctx, []string{"running-one", "waiting-one", "idle-one", "error-one"})
	if len(before) != len(after) {
		t.Fatalf("row count changed: before %d, after %d", len(before), len(after))
	}
	for id, beforeBytes := range before {
		afterBytes, ok := after[id]
		if !ok {
			t.Fatalf("row %q disappeared", id)
		}
		if !bytes.Equal(beforeBytes, afterBytes) {
			t.Fatalf("row %q changed:\nbefore: %s\nafter:  %s", id, beforeBytes, afterBytes)
		}
	}
}

// TestSpaceNoopWhenSelectionAlreadyOnSoleAttentionRow proves that when the
// selected row is itself the only one needing attention, `space` leaves
// selection exactly where it is (the wrap search lands back on it), rather
// than treating that as "nothing needs attention" and misreporting ok.
func TestSpaceNoopWhenSelectionAlreadyOnSoleAttentionRow(t *testing.T) {
	m := Model{sessions: []store.Session{
		{ID: "a", Status: "running"},
		{ID: "b", Status: "waiting"},
		{ID: "c", Status: "idle"},
	}}
	m.selected = 1
	updated, _ := m.Update(key(" "))
	got := updated.(Model)
	if got.selected != 1 {
		t.Fatalf("selected = %d, want 1 (unchanged)", got.selected)
	}
}

// snapshotSessions reads each named session row and returns its JSON
// encoding, keyed by ID, so a caller can compare two snapshots byte-for-byte
// without hand-listing every store.Session field (several of which are
// slices/maps that reflect.DeepEqual would need special-casing for anyway).
func snapshotSessions(t *testing.T, db *store.Store, ctx context.Context, ids []string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte, len(ids))
	for _, id := range ids {
		session, err := db.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("GetSession(%q): %v", id, err)
		}
		encoded, err := json.Marshal(session)
		if err != nil {
			t.Fatalf("marshal session %q: %v", id, err)
		}
		out[id] = encoded
	}
	return out
}
