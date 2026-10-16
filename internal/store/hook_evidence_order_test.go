package store

import "testing"

// R303 / SPEC §7: hook_at is the time of the NEWEST hook evidence. Hook
// deliveries can arrive out of order (one hook process per event, racing for
// the store), so a delayed repeat dated before a repeat the store already
// applied must not move hook_at backwards and age a hook that is still fresh.

const (
	orderStale = int64(1_000)
	orderNewer = int64(11_000)
	orderOlder = int64(9_000)
)

// reorderedRepeats seeds a hook-sourced row in status at status_at 1000, then
// delivers same-status hook repeats dated at each of ats, in that order.
func reorderedRepeats(t *testing.T, status, reason string, ats ...int64) *Store {
	t.Helper()
	db := openRepeatStore(t)
	seedRepeatRow(t, db, status, "hook", reason, true)
	for _, at := range ats {
		writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: status, Reason: reason, Source: "hook", At: at, EventKind: "repeat"})
	}
	return db
}

// Finding case: repeats at 11000 then 9000; a probe at 11500 (500 ms after
// the newest hook, stale_after 1000) loses.
func TestDelayedRepeatKeepsNewestHookEvidenceAgainstProbe(t *testing.T) {
	db := reorderedRepeats(t, "running", "prompt", orderNewer, orderOlder)
	if row := rowOf(t, db); row.HookAt != orderNewer {
		t.Fatalf("delayed repeat moved hook_at to %d, want the newest %d", row.HookAt, orderNewer)
	}
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: orderNewer + 500, StaleAfter: orderStale, EventKind: "probe.idle"})
	if row := rowOf(t, db); row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("a probe 500ms after the newest hook won: %q/%q", row.Status, row.StatusSource)
	}
}

// Finding case: the same reorder over idle; movement at 11500 loses.
func TestDelayedRepeatKeepsNewestHookEvidenceAgainstMovement(t *testing.T) {
	db := reorderedRepeats(t, "idle", "ready", orderNewer, orderOlder)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "output movement", Source: "movement",
		At: orderNewer + 500, StaleAfter: orderStale, EventKind: "movement.running", ExpectedStatus: "idle"})
	if row := rowOf(t, db); row.Status != "idle" || row.StatusSource != "hook" {
		t.Fatalf("movement 500ms after the newest hook won: %q/%q", row.Status, row.StatusSource)
	}
}

// Not in the finding: a waiting (attention) row, three repeats delivered
// newest-first-then-in-between; hook_at stays the newest, a probe inside the
// window loses, and R303 transition/seen/notification state is untouched.
func TestDelayedWaitingRepeatsKeepNewestEvidenceAndStayInert(t *testing.T) {
	db := reorderedRepeats(t, "waiting", "permission_prompt", orderNewer, orderOlder, orderNewer-500)
	row := requireInert(t, db, "hook", true)
	if row.HookAt != orderNewer || row.VerdictAt() != orderNewer {
		t.Fatalf("hook_at/VerdictAt = %d/%d, want %d", row.HookAt, row.VerdictAt(), orderNewer)
	}
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "working", Source: "probe",
		At: orderNewer + orderStale - 1, StaleAfter: orderStale, EventKind: "probe.running"})
	if row := rowOf(t, db); row.Status != "waiting" || row.StatusSource != "hook" {
		t.Fatalf("a probe inside the newest hook's window replaced waiting: %q/%q", row.Status, row.StatusSource)
	}
}

// Not in the finding: past stale_after of the NEWEST hook (not merely of the
// delayed one) the hook is genuinely stale and sampling takes over.
func TestDelayedRepeatStaleByNewestHookPermitsSampling(t *testing.T) {
	db := reorderedRepeats(t, "running", "prompt", orderNewer, orderOlder)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: orderNewer + orderStale, StaleAfter: orderStale, EventKind: "probe.idle"})
	if row := rowOf(t, db); row.Status != "idle" || row.StatusSource != "probe" {
		t.Fatalf("a probe at stale_after past the newest hook lost: %q/%q", row.Status, row.StatusSource)
	}

	db = reorderedRepeats(t, "idle", "ready", orderNewer, orderOlder)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "output movement", Source: "movement",
		At: orderNewer + orderStale, StaleAfter: orderStale, EventKind: "movement.running", ExpectedStatus: "idle"})
	if row := rowOf(t, db); row.Status != "running" || row.StatusSource != "movement" {
		t.Fatalf("movement at stale_after past the newest hook lost: %q/%q", row.Status, row.StatusSource)
	}
}

// Not in the finding: a delayed hook TRANSITION (a different status) dated
// before the newest hook repeat also leaves hook_at at the newest evidence.
func TestDelayedHookTransitionKeepsNewestHookEvidence(t *testing.T) {
	db := reorderedRepeats(t, "running", "prompt", orderNewer)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "stop", Source: "hook", At: orderOlder, EventKind: "stop"})
	row := rowOf(t, db)
	if row.Status != "idle" || row.HookAt != orderNewer {
		t.Fatalf("status %q hook_at %d, want idle at hook_at %d", row.Status, row.HookAt, orderNewer)
	}
}

// Not in the finding: once a probe owns the row, a hook taking it back starts
// the hook clock at its own time; it does not inherit an older hook's stamp
// and does not keep a newer one from a hook episode that already lost.
func TestHookTakingOverFromProbeStartsItsOwnClock(t *testing.T) {
	db := reorderedRepeats(t, "running", "prompt", orderNewer)
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "idle", Reason: "ready", Source: "probe",
		At: orderNewer + 5*orderStale, StaleAfter: orderStale, EventKind: "probe.idle"})
	writeAs(t, db, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "prompt", Source: "hook", At: orderOlder, EventKind: "late"})
	if row := rowOf(t, db); row.StatusSource != "hook" || row.HookAt != orderOlder {
		t.Fatalf("source %q hook_at %d, want hook at its own %d", row.StatusSource, row.HookAt, orderOlder)
	}
}
