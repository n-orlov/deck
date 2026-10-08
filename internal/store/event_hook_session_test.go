package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

// R233a: the session's own event-hook controls round-trip through
// CreateSession, SetEventHook and the Session read, with no dirty flag.
func TestSessionEventHookControlsRoundTrip(t *testing.T) {
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	on := true
	created, err := db.CreateSession(ctx, CreateSessionInput{
		ID: "h1", Name: "h1", CWD: "/w", Agent: "shell", CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1,
		EventHookEnabled: &on, EventHookEvents: []string{"waiting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.EventHookEnabled == nil || !*created.EventHookEnabled || !reflect.DeepEqual(created.EventHookEvents, []string{"waiting"}) {
		t.Fatalf("created row = %v / %#v", created.EventHookEnabled, created.EventHookEvents)
	}

	off := false
	if err := db.SetEventHook(ctx, "h1", &off, []string{}, "", 5); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetSession(ctx, "h1")
	if got.EventHookEnabled == nil || *got.EventHookEnabled || got.EventHookEvents == nil || len(got.EventHookEvents) != 0 {
		t.Fatalf("after SetEventHook = %v / %#v", got.EventHookEnabled, got.EventHookEvents)
	}
	if got.EnvDirty || got.LaunchDirty {
		t.Fatalf("SetEventHook set a dirty flag: %+v", got)
	}
	var kind string
	if err := db.DB().QueryRow(`SELECT kind FROM events WHERE session_id = 'h1' ORDER BY seq DESC LIMIT 1`).Scan(&kind); err != nil || kind != "set_event_hook" {
		t.Fatalf("last event kind = %q, %v; want set_event_hook", kind, err)
	}

	if err := db.SetEventHook(ctx, "h1", nil, nil, "user", 6); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetSession(ctx, "h1")
	if got.EventHookEnabled != nil || got.EventHookEvents != nil {
		t.Fatalf("inherit = %v / %#v", got.EventHookEnabled, got.EventHookEvents)
	}
	if err := db.SetEventHook(ctx, "", nil, nil, "user", 7); err == nil {
		t.Fatal("empty id accepted")
	}
	if err := db.SetEventHook(ctx, "missing", nil, nil, "user", 7); err == nil {
		t.Fatal("unknown session accepted")
	}

	// A damaged list reads as inherit, never as "offer nothing".
	if _, err := db.DB().Exec(`UPDATE sessions SET event_hook_events = 'not json' WHERE id = 'h1'`); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetSession(ctx, "h1")
	if got.EventHookEvents != nil {
		t.Fatalf("damaged list read as %#v, want inherit", got.EventHookEvents)
	}
}
