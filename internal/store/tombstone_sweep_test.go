package store

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// sweepGrace is a fixed delete-grace window used across this file's tests,
// distinct from the real DECK_DELETE_GRACE_MS default so a failing
// assertion cannot be confused with that default leaking in.
const sweepGrace = 5 * time.Minute

// TestSweepTombstonesReapsExpiredLeavesFresh is task 010's primary sweep
// proof: a tombstone whose deleted_at is strictly older than deleteGrace
// (as measured against the `now` argument, never wall-clock time) is
// reaped -- row, events and name freed -- while a tombstone still inside
// the grace window survives untouched, in the SAME call.
func TestSweepTombstonesReapsExpiredLeavesFresh(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	const now int64 = 1_700_000_000_000
	cutoff := now - sweepGrace.Milliseconds()

	expired := createTombstoneTestSession(t, st, ctx, "sweep-expired")
	if err := st.SoftDeleteSession(ctx, expired.ID, cutoff-1); err != nil {
		t.Fatalf("soft delete expired: %v", err)
	}
	fresh := createTombstoneTestSession(t, st, ctx, "sweep-fresh")
	if err := st.SoftDeleteSession(ctx, fresh.ID, cutoff+1); err != nil {
		t.Fatalf("soft delete fresh: %v", err)
	}

	if err := st.SweepTombstones(ctx, sweepGrace, now); err != nil {
		t.Fatalf("SweepTombstones: %v", err)
	}

	if _, err := st.GetSession(ctx, expired.ID); err == nil {
		t.Fatal("expired tombstone survived the sweep, want it reaped")
	}
	got, err := st.GetSession(ctx, fresh.ID)
	if err != nil {
		t.Fatalf("fresh tombstone was reaped, want it left alone: %v", err)
	}
	if got.DeletedAt != cutoff+1 {
		t.Fatalf("fresh tombstone DeletedAt = %d, want unchanged %d", got.DeletedAt, cutoff+1)
	}

	var reapedEvents int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id IS NULL AND kind = 'reaped' AND payload = ?`, expired.ID).Scan(&reapedEvents); err != nil {
		t.Fatal(err)
	}
	if reapedEvents != 1 {
		t.Fatalf("orphan 'reaped' events for %q = %d, want 1", expired.ID, reapedEvents)
	}

	// The name is free again -- a real CreateSession can reuse it.
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "sweep-expired-reused", Name: expired.Name, CWD: "/work/sweep-expired-reused",
		Agent: "shell", CapturedPath: "/bin", StatusAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session reusing swept name %q: %v", expired.Name, err)
	}
}

// TestSweepTombstonesThrottlesToAtMostOnceAnHour mirrors
// TestEnforceEventRetentionThrottlesToAtMostOnceAnHour exactly, applied to
// the tombstone sweep's own ui_state throttle key: the first call always
// runs (no prior ui_state row), a call minutes later against a fresh
// batch of expired tombstones is a no-op, and a call more than an hour
// after the first runs again and catches up.
func TestSweepTombstonesThrottlesToAtMostOnceAnHour(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	const firstRun int64 = 1_700_000_000_000
	cutoffAtFirstRun := firstRun - sweepGrace.Milliseconds()

	first := createTombstoneTestSession(t, st, ctx, "sweep-throttle-first")
	if err := st.SoftDeleteSession(ctx, first.ID, cutoffAtFirstRun-1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := st.SweepTombstones(ctx, sweepGrace, firstRun); err != nil {
		t.Fatalf("first SweepTombstones: %v", err)
	}
	if _, err := st.GetSession(ctx, first.ID); err == nil {
		t.Fatal("first (store-open) sweep must have reaped the already-expired tombstone")
	}

	// 10 minutes later: a fresh tombstone, already expired by the grace
	// window, is seeded, then SweepTombstones is called again. The
	// throttle must make this a no-op: the row must still be there.
	secondRun := firstRun + 10*time.Minute.Milliseconds()
	cutoffAtSecondRun := secondRun - sweepGrace.Milliseconds()
	second := createTombstoneTestSession(t, st, ctx, "sweep-throttle-second")
	if err := st.SoftDeleteSession(ctx, second.ID, cutoffAtSecondRun-1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := st.SweepTombstones(ctx, sweepGrace, secondRun); err != nil {
		t.Fatalf("second (throttled) SweepTombstones: %v", err)
	}
	if _, err := st.GetSession(ctx, second.ID); err != nil {
		t.Fatalf("second tombstone reaped despite the hourly throttle, want it left alone: %v", err)
	}

	// More than an hour after the FIRST run: the throttle lets a real pass
	// through again, and the row seeded above (already expired since the
	// second call) is finally reaped.
	thirdRun := firstRun + 61*time.Minute.Milliseconds()
	if err := st.SweepTombstones(ctx, sweepGrace, thirdRun); err != nil {
		t.Fatalf("third SweepTombstones: %v", err)
	}
	if _, err := st.GetSession(ctx, second.ID); err == nil {
		t.Fatal("third sweep (more than an hour after the first) must have reaped the row, throttle should have let it through")
	}
}

// TestSweepTombstonesIsCheapNoOpInsideInterval proves the throttled call
// really is a cheap no-op: it performs no write at all against ui_state
// (the last-run value stays exactly what the first call set) and touches
// no session row, not merely that the expired row it was handed survives.
func TestSweepTombstonesIsCheapNoOpInsideInterval(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	const firstRun int64 = 1_700_000_000_000
	if err := st.SweepTombstones(ctx, sweepGrace, firstRun); err != nil {
		t.Fatalf("first SweepTombstones: %v", err)
	}
	lastRunAfterFirst, err := st.getUIState(ctx, tombstoneSweepLastRunKey, "0")
	if err != nil {
		t.Fatal(err)
	}

	secondRun := firstRun + 30*time.Minute.Milliseconds()
	expired := createTombstoneTestSession(t, st, ctx, "sweep-noop-expired")
	if err := st.SoftDeleteSession(ctx, expired.ID, secondRun-sweepGrace.Milliseconds()-1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := st.SweepTombstones(ctx, sweepGrace, secondRun); err != nil {
		t.Fatalf("throttled SweepTombstones: %v", err)
	}

	lastRunAfterSecond, err := st.getUIState(ctx, tombstoneSweepLastRunKey, "0")
	if err != nil {
		t.Fatal(err)
	}
	if lastRunAfterSecond != lastRunAfterFirst {
		t.Fatalf("ui_state last-run = %q after the throttled call, want unchanged %q (no write should happen inside the interval)", lastRunAfterSecond, lastRunAfterFirst)
	}
	if _, err := st.GetSession(ctx, expired.ID); err != nil {
		t.Fatalf("throttled call reaped a row, want it untouched: %v", err)
	}
}

// TestSweepTombstonesHonoursFrozenClock proves the sweep never leaks a
// real time.Now() call: driven entirely by a `now` far in the future (long
// past any real wall-clock moment this test could ever run at) and a
// tombstone whose deleted_at sits just inside the grace window measured
// against THAT now, the row must survive -- if the sweep instead compared
// against the real clock, deleted_at (an ordinary, much smaller int64)
// would look ancient and get reaped regardless.
func TestSweepTombstonesHonoursFrozenClock(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	// Far beyond any real wall-clock UnixMilli value this test could ever
	// observe (year ~5138), so a leaked time.Now() comparison would behave
	// completely differently from the frozen `now` below.
	const frozenNow int64 = 100_000_000_000_000
	session := createTombstoneTestSession(t, st, ctx, "sweep-frozen")
	deletedAt := frozenNow - sweepGrace.Milliseconds() + 1000 // just inside the grace window
	if err := st.SoftDeleteSession(ctx, session.ID, deletedAt); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if err := st.SweepTombstones(ctx, sweepGrace, frozenNow); err != nil {
		t.Fatalf("SweepTombstones: %v", err)
	}

	got, err := st.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("row reaped despite being inside the grace window measured against the injected `now`: %v", err)
	}
	if got.DeletedAt != deletedAt {
		t.Fatalf("DeletedAt = %d, want unchanged %d", got.DeletedAt, deletedAt)
	}
}

// TestSweepTombstonesRejectsInvalidArgs guards SweepTombstones' own input
// validation, mirroring EnforceEventRetention's equivalent guard.
func TestSweepTombstonesRejectsInvalidArgs(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	if err := st.SweepTombstones(ctx, -time.Second, 1_700_000_000_000); err == nil {
		t.Fatal("SweepTombstones(deleteGrace<0) = nil error, want an error")
	}
	if err := st.SweepTombstones(ctx, sweepGrace, 0); err == nil {
		t.Fatal("SweepTombstones(now=0) = nil error, want an error")
	}
}

// TestSweepTombstonesLoopsAcrossMultipleBatches proves "bounded batches"
// means the loop keeps going until the backlog clears, not "one batch no
// matter how much is due" -- mirroring
// TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows'
// scale strategy but sized against tombstoneSweepBatchRows (200) instead
// of the events retention constant.
func TestSweepTombstonesLoopsAcrossMultipleBatches(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	const now int64 = 1_700_000_000_000
	cutoff := now - sweepGrace.Milliseconds()

	const expiredCount = 450 // more than 2x tombstoneSweepBatchRows (200)
	for i := 0; i < expiredCount; i++ {
		id := "sweep-batch-expired-" + strconv.Itoa(i)
		s := createTombstoneTestSession(t, st, ctx, id)
		if err := st.SoftDeleteSession(ctx, s.ID, cutoff-int64(i)-1); err != nil {
			t.Fatalf("soft delete %q: %v", id, err)
		}
	}
	fresh := createTombstoneTestSession(t, st, ctx, "sweep-batch-fresh")
	if err := st.SoftDeleteSession(ctx, fresh.ID, cutoff+1); err != nil {
		t.Fatalf("soft delete fresh: %v", err)
	}

	if err := st.SweepTombstones(ctx, sweepGrace, now); err != nil {
		t.Fatalf("SweepTombstones: %v", err)
	}

	var remaining int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE deleted_at != 0 AND id != ?`, fresh.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("tombstoned rows remaining after the sweep (excluding the fresh one) = %d, want 0 (all %d expired rows across multiple batches must be gone)", remaining, expiredCount)
	}
	if _, err := st.GetSession(ctx, fresh.ID); err != nil {
		t.Fatalf("fresh tombstone was swept up, want it left alone: %v", err)
	}
}
