package store

import (
	"context"
	"testing"
)

// R303 / SPEC §7: hook-evidence freshness is its own clock (sessions.hook_at),
// not status_at. A hook repeating the current status leaves the transition
// state alone (status_at, seen, notify_epoch, hook_fired) yet is fresh hook
// evidence, so no probe and no movement write may replace it inside
// stale_after of that repeat. The atomic store guards enforce it, not only the
// reconciler's eligibility pre-check.

const (
	hookStale  = int64(10_000)
	hookRepeat = repeatSeedAt + 50_000 // far past stale_after of the seeded transition
)

func writeAs(t *testing.T, db *Store, in StatusUpdateInput) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

// repeatHook seeds a hook-sourced row in status at repeatSeedAt and then has a
// hook repeat that status at hookRepeat.
func repeatHook(t *testing.T, status, reason string) *Store {
	t.Helper()
	db := openRepeatStore(t)
	seedRepeatRow(t, db, status, "hook", reason, true)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: status, Reason: reason, Source: "hook", At: hookRepeat, EventKind: "repeat"})
	return db
}

func rowOf(t *testing.T, db *Store) Session {
	t.Helper()
	row, _ := readRepeatRow(t, db)
	return row
}

// Finding case: a probe just after a repeated hook loses, although the
// transition itself is older than stale_after.
func TestFreshRepeatHookBeatsAProbe(t *testing.T) {
	db := repeatHook(t, "running", "prompt")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + 1_000, StaleAfter: hookStale, EventKind: "probe.idle"})
	row := rowOf(t, db)
	if row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("a probe replaced a just-repeated hook: status %q source %q", row.Status, row.StatusSource)
	}
}

// Finding case: the movement fallback over a repeated idle hook loses too.
func TestFreshRepeatHookBeatsMovement(t *testing.T) {
	db := repeatHook(t, "idle", "ready")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "output movement", Source: "movement",
		At: hookRepeat + 1_000, StaleAfter: hookStale, EventKind: "movement.running", ExpectedStatus: "idle"})
	row := rowOf(t, db)
	if row.Status != "idle" || row.StatusSource != "hook" {
		t.Fatalf("movement replaced a just-repeated hook: status %q source %q", row.Status, row.StatusSource)
	}
}

// Not in the finding: a repeated waiting hook (an attention status) is fresh
// evidence as well, and a probe cannot clear it.
func TestFreshRepeatWaitingHookBeatsAProbe(t *testing.T) {
	db := repeatHook(t, "waiting", "permission_prompt")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "working", Source: "probe",
		At: hookRepeat + 9_999, StaleAfter: hookStale, EventKind: "probe.running"})
	if row := rowOf(t, db); row.Status != "waiting" || row.StatusSource != "hook" {
		t.Fatalf("a probe replaced a just-repeated waiting hook: %q/%q", row.Status, row.StatusSource)
	}
}

// Not in the finding: a repeat that repeats over a probe-sourced row hands the
// row to the hook source and it is fresh from the repeat, not from the older
// transition.
func TestHookRepeatOverProbeRowIsFresh(t *testing.T) {
	db := openRepeatStore(t)
	seedRepeatRow(t, db, "running", "probe", "working", true)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "prompt", Source: "hook", At: hookRepeat, EventKind: "repeat"})
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + 1, StaleAfter: hookStale, EventKind: "probe.idle"})
	if row := rowOf(t, db); row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("probe beat a repeat hook that took the row over: %q/%q", row.Status, row.StatusSource)
	}
}

// Not in the finding: successive repeats keep extending freshness.
func TestEachRepeatHookExtendsFreshness(t *testing.T) {
	db := repeatHook(t, "running", "prompt")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "prompt", Source: "hook", At: hookRepeat + 8_000, EventKind: "repeat"})
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + 16_000, StaleAfter: hookStale, EventKind: "probe.idle"})
	if row := rowOf(t, db); row.Status != "running" {
		t.Fatalf("second repeat did not extend freshness: %q/%q", row.Status, row.StatusSource)
	}
}

// R303 is unchanged by tracking the hook clock: a repeat still leaves the
// transition stamp, seen state, notification epoch and hook_fired alone, and
// hook_at carries the repeat's own time.
func TestRepeatHookAdvancesOnlyHookEvidence(t *testing.T) {
	db := repeatHook(t, "waiting", "permission_prompt")
	row := requireInert(t, db, "hook", true)
	if row.HookAt != hookRepeat {
		t.Fatalf("hook_at = %d, want %d", row.HookAt, hookRepeat)
	}
	if row.VerdictAt() != hookRepeat {
		t.Fatalf("VerdictAt = %d, want the repeat's %d", row.VerdictAt(), hookRepeat)
	}
}

// Genuinely stale hooks still permit sampling and fallback.
func TestStaleHookStillPermitsProbeAndMovement(t *testing.T) {
	db := repeatHook(t, "running", "prompt")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + hookStale, StaleAfter: hookStale, EventKind: "probe.idle"})
	if row := rowOf(t, db); row.Status != "idle" || row.StatusSource != "probe" {
		t.Fatalf("a probe at exactly stale_after after the hook lost: %q/%q", row.Status, row.StatusSource)
	}

	db = repeatHook(t, "idle", "ready")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "output movement", Source: "movement",
		At: hookRepeat + hookStale + 1, StaleAfter: hookStale, EventKind: "movement.running", ExpectedStatus: "idle"})
	if row := rowOf(t, db); row.Status != "running" || row.StatusSource != "movement" {
		t.Fatalf("movement past the repeat's stale_after lost: %q/%q", row.Status, row.StatusSource)
	}
}

// A hook older than the repeat does not borrow its freshness backwards, and a
// non-hook write never advances hook evidence.
func TestSampledWritesNeverAdvanceHookEvidence(t *testing.T) {
	db := repeatHook(t, "running", "prompt")
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + 20_000, StaleAfter: hookStale, EventKind: "probe.idle"})
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: hookRepeat + 30_000, StaleAfter: hookStale, EventKind: "probe.idle"})
	row := rowOf(t, db)
	if row.HookAt != hookRepeat {
		t.Fatalf("probe writes moved hook_at to %d", row.HookAt)
	}
	// The probe-sourced row is not hook-fresh any more: a movement write is
	// judged by the stored source, not by the old hook_at.
	if row.VerdictAt() != row.StatusAt {
		t.Fatalf("VerdictAt = %d on a probe-sourced row, want status_at %d", row.VerdictAt(), row.StatusAt)
	}
}

// A hook that lost precedence leaves no hook evidence behind.
func TestVetoedHookLeavesNoEvidence(t *testing.T) {
	db := openRepeatStore(t)
	seedRepeatRow(t, db, "running", "hook", "prompt", true)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "stopped", Reason: "killed", Source: "user", KilledByUser: true, At: 2_000, EventKind: "stop"})
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "prompt", Source: "hook", At: hookRepeat, EventKind: "late"})
	if row := rowOf(t, db); row.HookAt == hookRepeat || row.Status != "stopped" {
		t.Fatalf("vetoed hook left evidence or a verdict: hook_at=%d status=%q", row.HookAt, row.Status)
	}
}

// schemaV20 dates a hook-sourced row by status_at and leaves others at 0.
func TestSchemaV20BackfillsHookAt(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	for id, source := range map[string]string{"a": "hook", "b": "probe"} {
		if _, err := db.CreateSession(ctx, CreateSessionInput{ID: id, Name: id, CWD: "/w", Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: source, StatusAt: 4242, CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.ExecContext(ctx, `UPDATE sessions SET hook_at = 0 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range schemaV20[1:] {
		if _, err := db.db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]int64{"a": 4242, "b": 0} {
		got, err := db.GetSession(ctx, id)
		if err != nil || got.HookAt != want {
			t.Fatalf("session %s hook_at = %d (%v), want %d", id, got.HookAt, err, want)
		}
	}
}
