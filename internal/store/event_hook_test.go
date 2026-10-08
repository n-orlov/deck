package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEventHookStateReadsTheSessionColumns(t *testing.T) {
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	newLeaseTestSession(t, db, "s1", "running")

	state, err := db.EventHookState(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled != nil || state.Events != nil || state.Fired != "" || state.Important || state.Sensitive {
		t.Fatalf("a fresh row must inherit everything: %+v", state)
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET event_hook_enabled = 0, event_hook_events = '["idle","error"]',
		hook_fired = '[{"kind":"idle","reason":""}]', important = 1, sensitive = 1 WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	state, err = db.EventHookState(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled == nil || *state.Enabled || !reflect.DeepEqual(state.Events, []string{"idle", "error"}) ||
		state.Fired == "" || !state.Important || !state.Sensitive {
		t.Fatalf("state = %+v", state)
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET event_hook_enabled = 1, event_hook_events = '[]' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if state, _ = db.EventHookState(ctx, "s1"); state.Enabled == nil || !*state.Enabled || state.Events == nil || len(state.Events) != 0 {
		t.Fatalf("an empty list must stay a real value: %+v", state)
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET event_hook_events = 'not json' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if state, _ = db.EventHookState(ctx, "s1"); state.Events != nil {
		t.Fatalf("a damaged list must inherit: %+v", state)
	}
	if _, err := db.EventHookState(ctx, "missing"); err == nil {
		t.Fatal("a missing session must be an error")
	}
}

func TestClaimHookFiredIsACompareAndSet(t *testing.T) {
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	newLeaseTestSession(t, db, "s1", "running")

	won, err := db.ClaimHookFired(ctx, "s1", "", `[{"kind":"idle","reason":""}]`)
	if err != nil || !won {
		t.Fatalf("first claim = %v, %v, want won", won, err)
	}
	if won, err = db.ClaimHookFired(ctx, "s1", "", `[{"kind":"error","reason":""}]`); err != nil || won {
		t.Fatalf("a second claim from the same stale read = %v, %v, want lost", won, err)
	}
	state, _ := db.EventHookState(ctx, "s1")
	if state.Fired != `[{"kind":"idle","reason":""}]` {
		t.Fatalf("the loser overwrote hook_fired: %q", state.Fired)
	}
}
