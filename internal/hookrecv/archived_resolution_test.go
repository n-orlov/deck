package hookrecv

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestReceiveResolvesAnArchivedRowByBothKeys is R71 leg 3's own test and the
// direct reproduction of issue #8's captured banner:
//
//	deck hook: hook session could not be resolved
//	  (conversation_id="78e4cab4-..." injected_session_id="0aee5fc4-...")
//
// Both keys named the right row; resolution failed anyway, because it read
// ListSessions -- the sidebar's query, which also demands archived_at = 0.
// An archived row is hidden, not disowned, so its hooks must land ON it.
//
// This assertion stays valuable even though R71 leg 1 stops a resume from
// producing an archived row with a live pane: service.Archive kills the pane
// and THEN sets archived_at, so a hook already in flight across that boundary
// still arrives after the flag is set and would still be orphaned today.
func TestReceiveResolvesAnArchivedRowByBothKeys(t *testing.T) {
	db := newHookStore(t)
	ctx := context.Background()
	createHookSession(t, db, "archived-row", "claude", "archived-conversation")
	if err := db.ArchiveSession(ctx, "archived-row", 5); err != nil {
		t.Fatal(err)
	}
	// Precondition: the row really is invisible to the display query, so the
	// test cannot pass by accident of an unarchived fixture.
	visible, err := db.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("fixture row is still in the default list: %#v", visible)
	}

	// Key 1: the payload's own conversation id (SPEC §8.1's first route).
	result, err := Receive(ctx, db, []byte(`{"hook_event_name":"SessionStart","session_id":"archived-conversation","source":"resume"}`), "", "", 10)
	if err != nil {
		t.Fatalf("conversation-id hook on an archived row: %v", err)
	}
	if result.Orphan || result.SessionID != "archived-row" {
		t.Fatalf("conversation-id resolution = %#v, want archived-row, not an orphan", result)
	}
	row, err := db.GetSession(ctx, "archived-row")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("archived row after SessionStart = %#v, want running/hook", row)
	}
	if row.ArchivedAt == 0 {
		t.Fatal("resolution cleared archived_at; the flag must be untouched")
	}

	// Key 2: the deck row id injected into the pane environment (§8.1's
	// fallback), with a conversation id that matches no row.
	result, err = Receive(ctx, db, []byte(`{"hook_event_name":"Notification","notification_type":"question","session_id":"not-a-known-conversation"}`), "archived-row", "", 11)
	if err != nil {
		t.Fatalf("injected-id hook on an archived row: %v", err)
	}
	if result.Orphan || result.SessionID != "archived-row" {
		t.Fatalf("injected-id resolution = %#v, want archived-row, not an orphan", result)
	}

	// Both hooks are recorded AGAINST the row: no orphan event exists.
	var orphans int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id IS NULL`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("orphan events = %d, want 0", orphans)
	}
	var recorded int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind IN ('session_start','notification')`, "archived-row").Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 2 {
		t.Fatalf("hook events on the archived row = %d, want 2", recorded)
	}
}

// TestReceiveKeepsATombstonedRowsHookAnOrphan is leg 3's negative half: the
// lazy fix drops BOTH terms of ListSessions' WHERE clause. deleted_at = 0
// stays, so a row awaiting the reaper is still not a hook target and its
// payload is preserved as an orphan (session_id NULL) exactly as before.
func TestReceiveKeepsATombstonedRowsHookAnOrphan(t *testing.T) {
	db := newHookStore(t)
	ctx := context.Background()
	createHookSession(t, db, "tombstoned-row", "claude", "tombstoned-conversation")
	if err := db.SoftDeleteSession(ctx, "tombstoned-row", 5); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name        string
		raw         string
		injected    string
		wantOrphans int
	}{
		{name: "by conversation id", raw: `{"hook_event_name":"Stop","session_id":"tombstoned-conversation"}`, wantOrphans: 1},
		{name: "by injected row id", raw: `{"hook_event_name":"Stop","session_id":"unknown"}`, injected: "tombstoned-row", wantOrphans: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Receive(ctx, db, []byte(tc.raw), tc.injected, "", 20+int64(tc.wantOrphans))
			if !errors.Is(err, ErrUnresolved) || !result.Orphan || result.SessionID != "" {
				t.Fatalf("tombstoned hook = %#v, err %v; want unresolved orphan", result, err)
			}
			var orphans int
			if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id IS NULL`).Scan(&orphans); err != nil {
				t.Fatal(err)
			}
			if orphans != tc.wantOrphans {
				t.Fatalf("orphan events = %d, want %d", orphans, tc.wantOrphans)
			}
			var onRow sql.NullInt64
			if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, "tombstoned-row").Scan(&onRow); err != nil {
				t.Fatal(err)
			}
			if onRow.Int64 != 1 { // the "deleted" tombstone event only
				t.Fatalf("events on the tombstoned row = %d, want only its own deleted event", onRow.Int64)
			}
			row, err := db.GetSession(ctx, "tombstoned-row")
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != "starting" {
				t.Fatalf("tombstoned row status = %q, want starting (untouched)", row.Status)
			}
		})
	}
}
