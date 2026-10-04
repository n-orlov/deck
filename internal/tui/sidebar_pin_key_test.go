package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 010's own acceptance test obligation (SPEC §11's pin
// rule, R159/PRD): a bare top-level `p` toggles the sidebar pin, singly or
// over the marked set, entirely through a returned tea.Cmd
// (setSessionsPinnedCmd, tui.go) -- Update itself never calls the store
// directly for this key, only decides which ids and which direction. Every
// test below runs the real store path (store.OpenPath, never a stub), and
// proves the write happens only once the returned command actually runs,
// never synchronously inside Update.

// sidebarPinTestStore opens a real, temp-home store and returns it
// alongside a helper that creates a session row.
func sidebarPinTestStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sidebarPinCreateSession(t *testing.T, db *store.Store, id, name string, createdAt int64) {
	t.Helper()
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: "/work/" + id, Agent: "shell", CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: createdAt, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("create session %q: %v", id, err)
	}
}

// TestPTogglesPin proves R159's core: `p` on an unpinned row pins it --
// pinned_at becomes non-zero once the returned tea.Cmd actually runs, and
// Update itself, before that command runs, has performed NO store write at
// all (the row is still unpinned in the store the instant Update returns).
// A follow-up reload (sessionsLoaded, driven by loadSessions the pin
// message's own case schedules) re-sorts the pinned row to the top of its
// group. `p` again unpins it the same way.
func TestPTogglesPin(t *testing.T) {
	db := sidebarPinTestStore(t)
	sidebarPinCreateSession(t, db, "s1", "alpha", 100)
	sidebarPinCreateSession(t, db, "s2", "beta", 200)

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	model := NewWithShellCreator(db, config.Settings{Clock: clock, SortOrder: SortOrderCreated}, "", nil)
	updated, _ := model.Update(model.loadSessions())
	model = updated.(Model)

	// alpha (created first) sorts SECOND under "created" order (newest
	// first), so pinning the OLDER row on purpose proves the reload moves
	// it, which "created" order alone would never do on its own.
	model.selected = rowCursor(1)
	if model.sessions[0].ID != "s2" || model.sessions[1].ID != "s1" {
		t.Fatalf("sessions after initial load = %#v, want s2 then s1 under created order", model.sessions)
	}

	got, cmd := model.Update(key("p"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("p returned no tea.Cmd")
	}
	// Update itself performs no store write: the row must still read
	// unpinned in the store the instant Update returns, before cmd ever
	// runs.
	row, err := db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt != 0 {
		t.Fatalf("Update(p) itself wrote pinned_at = %d before its returned Cmd ever ran; Update must perform no store write", row.PinnedAt)
	}

	msg := cmd()
	pinnedMsg, ok := msg.(sessionsPinned)
	if !ok {
		t.Fatalf("cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("pin cmd errored: %v", pinnedMsg.err)
	}

	// The store write happened once the Cmd ran, above.
	row, err = db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt == 0 {
		t.Fatal("running p's own tea.Cmd did not pin s1")
	}

	got, reloadCmd := model.Update(pinnedMsg)
	model = got.(Model)
	if reloadCmd == nil {
		t.Fatal("sessionsPinned scheduled no reload")
	}
	got, _ = model.Update(reloadCmd())
	model = got.(Model)

	if len(model.sessions) != 2 || model.sessions[0].ID != "s1" {
		t.Fatalf("sessions after reload = %#v, want s1 (now pinned) first", model.sessions)
	}
	if model.sessions[0].PinnedAt == 0 {
		t.Fatal("reloaded s1 does not carry its pin")
	}

	// p again on the now-pinned, now-top row unpins it.
	model.selected = rowCursor(0)
	got, cmd = model.Update(key("p"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("second p returned no tea.Cmd")
	}
	row, err = db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt == 0 {
		t.Fatal("Update(p) itself already unpinned s1 before its Cmd ran")
	}
	msg = cmd()
	pinnedMsg, ok = msg.(sessionsPinned)
	if !ok {
		t.Fatalf("second cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("unpin cmd errored: %v", pinnedMsg.err)
	}
	row, err = db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt != 0 {
		t.Fatalf("second p did not unpin s1, pinned_at = %d", row.PinnedAt)
	}
}

// TestPMarkedSetMixedPinsAllThenUnpinsAll proves R159's marked-set rule:
// with a mixed marked set (some pinned, some not), `p` pins every marked
// row; with every marked row now pinned, `p` again unpins every one of
// them. Marks themselves are left untouched by either press -- SPEC names
// no such side effect for `p`, unlike `x`/`dd`.
func TestPMarkedSetMixedPinsAllThenUnpinsAll(t *testing.T) {
	db := sidebarPinTestStore(t)
	sidebarPinCreateSession(t, db, "s1", "alpha", 100)
	sidebarPinCreateSession(t, db, "s2", "beta", 200)
	sidebarPinCreateSession(t, db, "s3", "gamma", 300)

	// s1 starts pinned, s2/s3 do not -- a genuinely mixed marked set.
	if err := db.SetSessionsPinned(context.Background(), []string{"s1"}, true, 50); err != nil {
		t.Fatal(err)
	}

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	model := NewWithShellCreator(db, config.Settings{Clock: clock}, "", nil)
	updated, _ := model.Update(model.loadSessions())
	model = updated.(Model)

	model.marked = map[string]bool{"s1": true, "s2": true, "s3": true}

	got, cmd := model.Update(key("p"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("marked-set p returned no tea.Cmd")
	}
	if len(model.marked) != 3 {
		t.Fatalf("marked set changed size on p: %#v", model.marked)
	}
	msg := cmd()
	pinnedMsg, ok := msg.(sessionsPinned)
	if !ok {
		t.Fatalf("cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("bulk pin cmd errored: %v", pinnedMsg.err)
	}
	for _, id := range []string{"s1", "s2", "s3"} {
		row, err := db.GetSession(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if row.PinnedAt == 0 {
			t.Fatalf("mixed marked-set p did not pin %q", id)
		}
	}

	got, _ = model.Update(pinnedMsg)
	model = got.(Model)

	// Reload so m.sessions reflects the pin the Cmd just wrote -- the
	// marked-set direction check below reads m.sessions/markedSessions(),
	// which would otherwise still show the pre-pin state.
	got, _ = model.Update(model.loadSessions())
	model = got.(Model)

	// Every marked row is now pinned: the next p must unpin the whole set.
	model.marked = map[string]bool{"s1": true, "s2": true, "s3": true}
	got, cmd = model.Update(key("p"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("all-pinned marked-set p returned no tea.Cmd")
	}
	if len(model.marked) != 3 {
		t.Fatalf("marked set changed size on the all-pinned p: %#v", model.marked)
	}
	msg = cmd()
	pinnedMsg, ok = msg.(sessionsPinned)
	if !ok {
		t.Fatalf("second cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("bulk unpin cmd errored: %v", pinnedMsg.err)
	}
	for _, id := range []string{"s1", "s2", "s3"} {
		row, err := db.GetSession(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if row.PinnedAt != 0 {
			t.Fatalf("all-pinned marked-set p did not unpin %q, pinned_at = %d", id, row.PinnedAt)
		}
	}
}

// TestPOnHeaderDoesNothingAndRecordsNoEvent proves the third leg of R159:
// `p` with the cursor on a group header is refused by the existing
// session_scoped_guard.go guard (its own generic table,
// TestSessionScopedKeysAreInertOnAHeader, already proves the no-mutation/
// no-Cmd half of this over a stub model) -- this test additionally proves
// the real store is never touched at all: no pinned_at write, and no
// events row, exactly as a pin/unpin never records one even when it does
// run (SetSessionsPinnedPinsAndUnpins, internal/store).
func TestPOnHeaderDoesNothingAndRecordsNoEvent(t *testing.T) {
	db := sidebarPinTestStore(t)
	sidebarPinCreateSession(t, db, "s1", "alpha", 100)

	groupID, err := db.CreateGroup(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSessionGroup(context.Background(), "s1", groupID.ID, "user", 150); err != nil {
		t.Fatal(err)
	}

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	model := NewWithShellCreator(db, config.Settings{Clock: clock}, "", nil)
	updated, _ := model.Update(model.loadSessions())
	model = updated.(Model)
	model.selected = headerCursor(groupID.ID)

	var eventsBefore int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}

	got, cmd := model.Update(key("p"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("p on a group header returned a non-nil tea.Cmd; it must be a pure no-op")
	}

	row, err := db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt != 0 {
		t.Fatalf("p on a group header wrote pinned_at = %d", row.PinnedAt)
	}

	var eventsAfter int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("p on a group header changed the events row count from %d to %d; it must record no event", eventsBefore, eventsAfter)
	}
}
