package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
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

func TestClaimHookPairMergesDistinctPairsAndRejectsOnlyTheSamePair(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	for _, c := range []struct {
		kind, reason string
		want         bool
	}{
		{"waiting", "permission", true},
		{"waiting", "permission", false},
		{"waiting", "question", true},
		{"error", "", true},
		{"error", "", false},
		{"waiting", "question", false},
	} {
		if won, err := db.ClaimHookPair(ctx, "s1", c.kind, c.reason); err != nil || won != c.want {
			t.Fatalf("claim (%s,%s) = %v, %v, want %v", c.kind, c.reason, won, err, c.want)
		}
	}
	state, _ := db.EventHookState(ctx, "s1")
	if want := `[{"kind":"waiting","reason":"permission"},{"kind":"waiting","reason":"question"},{"kind":"error","reason":""}]`; state.Fired != want {
		t.Fatalf("hook_fired = %q, want %q", state.Fired, want)
	}
	if _, err := db.ClaimHookPair(ctx, "missing", "idle", ""); err == nil {
		t.Fatal("a missing session must be an error")
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET hook_fired = 'not json' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if won, err := db.ClaimHookPair(ctx, "s1", "idle", ""); err != nil || !won {
		t.Fatalf("a damaged column must re-fire, got %v, %v", won, err)
	}
}

func TestClaimHookPairConcurrentClaims(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	const same, distinct = 8, 6
	var identicalWins, distinctWins atomic.Int32
	var wg sync.WaitGroup
	for range same {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if won, err := db.ClaimHookPair(ctx, "s1", "idle", ""); err != nil {
				t.Error(err)
			} else if won {
				identicalWins.Add(1)
			}
		}()
	}
	for i := range distinct {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if won, err := db.ClaimHookPair(ctx, "s1", "waiting", fmt.Sprintf("reason-%d", i)); err != nil {
				t.Error(err)
			} else if won {
				distinctWins.Add(1)
			}
		}()
	}
	wg.Wait()
	if identicalWins.Load() != 1 {
		t.Fatalf("identical pair won %d times, want exactly 1", identicalWins.Load())
	}
	if distinctWins.Load() != distinct {
		t.Fatalf("distinct reasons won %d times, want %d", distinctWins.Load(), distinct)
	}
	state, _ := db.EventHookState(ctx, "s1")
	var fired []struct{ Kind, Reason string }
	if err := json.Unmarshal([]byte(state.Fired), &fired); err != nil || len(fired) != distinct+1 {
		t.Fatalf("hook_fired %q holds %d pairs, want %d (%v)", state.Fired, len(fired), distinct+1, err)
	}
}

func TestClaimHookPairKeysOnKindAndReasonTogetherNotEitherAlone(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	// The same reason under a different kind is a different pair, and the
	// same kind with an empty reason is distinct from one with a reason.
	for _, c := range []struct {
		kind, reason string
		want         bool
	}{
		{"waiting", "permission", true},
		{"error", "permission", true},
		{"waiting", "", true},
		{"error", "permission", false},
		{"waiting", "", false},
	} {
		if won, err := db.ClaimHookPair(ctx, "s1", c.kind, c.reason); err != nil || won != c.want {
			t.Fatalf("claim (%s,%s) = %v, %v, want %v", c.kind, c.reason, won, err, c.want)
		}
	}
}

func TestClaimHookPairIsPerSessionAndConcurrentAcrossSessions(t *testing.T) {
	ctx := context.Background()
	db := openHookStateStore(t)
	newLeaseTestSession(t, db, "s2", "running")
	var wins atomic.Int32
	var wg sync.WaitGroup
	for _, id := range []string{"s1", "s2"} {
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if won, err := db.ClaimHookPair(ctx, id, "waiting", "question"); err != nil {
					t.Error(err)
				} else if won {
					wins.Add(1)
				}
			}()
		}
	}
	wg.Wait()
	if wins.Load() != 2 {
		t.Fatalf("the same pair won %d times across two sessions, want exactly one per session (2)", wins.Load())
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
