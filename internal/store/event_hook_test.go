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

func TestEventSeqReportsTheAppendedEventRow(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	var seq int64
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "s1", Status: "waiting", Reason: "p",
		Source: "hook", At: 2, EventKind: "notification", EventSeq: &seq}); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := db.DB().QueryRow(`SELECT kind FROM events WHERE seq = ?`, seq).Scan(&kind); err != nil || seq == 0 || kind != "notification" {
		t.Fatalf("seq %d -> kind %q, %v", seq, kind, err)
	}
}

func TestEventHookResultRoundTripsAndNeedsAnExistingEvent(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	var seq int64
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "s1", Status: "waiting", Reason: "p",
		Source: "hook", At: 2, EventKind: "notification", EventSeq: &seq}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.EventHookResultOf(ctx, seq); err != nil || ok {
		t.Fatalf("an event nothing was spawned for reads a result: ok=%v err=%v", ok, err)
	}
	want := EventHookResult{Kind: "waiting", ExitCode: -1, TimedOut: true, Output: "tail", Error: "boom"}
	if err := db.RecordEventHookResult(ctx, seq, want); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := db.EventHookResultOf(ctx, seq); err != nil || !ok || got != want {
		t.Fatalf("read back %+v ok=%v err=%v, want %+v", got, ok, err, want)
	}
	if err := db.RecordEventHookResult(ctx, seq+1000, want); err == nil {
		t.Fatal("recording against a missing event must fail, never insert one")
	}
	if _, ok, err := db.EventHookResultOf(ctx, seq+1000); err != nil || ok {
		t.Fatalf("missing event reads a result: ok=%v err=%v", ok, err)
	}
}

func openHookStateStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	newLeaseTestSession(t, db, "s1", "running")
	return db
}

func TestLastEventHookResultIsTheNewestRecordedOne(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	newLeaseTestSession(t, db, "s2", "running")
	if _, ok, err := db.LastEventHookResult(ctx, ""); err != nil || ok {
		t.Fatalf("no hook has run yet: ok=%v err=%v", ok, err)
	}
	record := func(session string, at int64, res EventHookResult) {
		t.Helper()
		var seq int64
		if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: session, Status: "waiting", Reason: "p",
			Source: "hook", At: at, EventKind: "notification", EventSeq: &seq}); err != nil {
			t.Fatal(err)
		}
		if err := db.RecordEventHookResult(ctx, seq, res); err != nil {
			t.Fatal(err)
		}
	}
	first := EventHookResult{Kind: "waiting", ExitCode: 3, Output: "boom"}
	second := EventHookResult{Kind: "idle", ExitCode: -1, TimedOut: true}
	record("s1", 10, first)
	record("s2", 20, second)
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'waiting' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	// An event with no spawn behind it never counts as a result.
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "s1", Status: "idle", Reason: "q",
		Source: "hook", At: 30, EventKind: "stop"}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.LastEventHookResult(ctx, "s1")
	if err != nil || !ok || got.EventHookResult != first || got.SessionID != "s1" || got.At != 10 {
		t.Fatalf("session s1: %+v ok=%v err=%v", got, ok, err)
	}
	got, ok, err = db.LastEventHookResult(ctx, "")
	if err != nil || !ok || got.EventHookResult != second || got.SessionID != "s2" || got.At != 20 {
		t.Fatalf("any session: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := db.LastEventHookResult(ctx, "nobody"); err != nil || ok {
		t.Fatalf("unknown session: ok=%v err=%v", ok, err)
	}
}
