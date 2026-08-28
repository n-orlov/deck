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

	if _, err := st.SweepTombstones(ctx, sweepGrace, now); err != nil {
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
	if _, err := st.SweepTombstones(ctx, sweepGrace, firstRun); err != nil {
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
	if _, err := st.SweepTombstones(ctx, sweepGrace, secondRun); err != nil {
		t.Fatalf("second (throttled) SweepTombstones: %v", err)
	}
	if _, err := st.GetSession(ctx, second.ID); err != nil {
		t.Fatalf("second tombstone reaped despite the hourly throttle, want it left alone: %v", err)
	}

	// More than an hour after the FIRST run: the throttle lets a real pass
	// through again, and the row seeded above (already expired since the
	// second call) is finally reaped.
	thirdRun := firstRun + 61*time.Minute.Milliseconds()
	if _, err := st.SweepTombstones(ctx, sweepGrace, thirdRun); err != nil {
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
	if _, err := st.SweepTombstones(ctx, sweepGrace, firstRun); err != nil {
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
	if _, err := st.SweepTombstones(ctx, sweepGrace, secondRun); err != nil {
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
// real time.Now() call, by driving it with two frozen clocks whose verdict
// is the OPPOSITE of what the real wall clock would decide, so neither
// case can pass under a leaked time.Now():
//
//   - a clock frozen in the past (2001) with a tombstone fresh against it:
//     the real clock (2024+) would see an ancient deleted_at and reap it,
//     so survival is only possible if `now` alone decided;
//   - a clock frozen far in the future (year ~5138) with a tombstone
//     expired against it: the real clock would see a deleted_at millennia
//     in the future, well inside any grace window, and leave it, so the
//     reap is only possible if `now` alone decided.
//
// Each case gets its own store, because the hourly ui_state throttle would
// otherwise make the second call a no-op for reasons unrelated to clocks.
// The persisted last-run value is checked too: a leak in the throttle write
// would stamp wall-clock time rather than the injected `now`.
func TestSweepTombstonesHonoursFrozenClock(t *testing.T) {
	t.Run("clock frozen in the past leaves a row the real clock would reap", func(t *testing.T) {
		st := openTombstoneTestStore(t)
		ctx := context.Background()

		// 2001-09-09, far behind any moment this test can really run at, so
		// the real clock's cutoff sits ~20 years past deletedAt below.
		const frozenNow int64 = 1_000_000_000_000
		if realNow := time.Now().UnixMilli(); realNow-frozenNow < sweepGrace.Milliseconds() {
			t.Fatalf("test premise broken: real clock %d is not far enough ahead of the frozen clock %d", realNow, frozenNow)
		}
		session := createTombstoneTestSession(t, st, ctx, "sweep-frozen-past")
		deletedAt := frozenNow - sweepGrace.Milliseconds() + 1000 // just inside the grace window
		if err := st.SoftDeleteSession(ctx, session.ID, deletedAt); err != nil {
			t.Fatalf("soft delete: %v", err)
		}

		if _, err := st.SweepTombstones(ctx, sweepGrace, frozenNow); err != nil {
			t.Fatalf("SweepTombstones: %v", err)
		}

		got, err := st.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("row reaped though it is inside the grace window measured against the injected `now` -- a real time.Now() leaked into the age comparison: %v", err)
		}
		if got.DeletedAt != deletedAt {
			t.Fatalf("DeletedAt = %d, want unchanged %d", got.DeletedAt, deletedAt)
		}
		assertSweepLastRun(t, st, ctx, frozenNow)
	})

	t.Run("clock frozen in the future reaps a row the real clock would keep", func(t *testing.T) {
		st := openTombstoneTestStore(t)
		ctx := context.Background()

		// Year ~5138: deletedAt below is itself in the far future, so a real
		// time.Now() comparison would put it comfortably inside any grace
		// window and reap nothing at all.
		const frozenNow int64 = 100_000_000_000_000
		if realNow := time.Now().UnixMilli(); realNow >= frozenNow-sweepGrace.Milliseconds() {
			t.Fatalf("test premise broken: real clock %d has caught up with the frozen clock %d", realNow, frozenNow)
		}
		session := createTombstoneTestSession(t, st, ctx, "sweep-frozen-future")
		deletedAt := frozenNow - sweepGrace.Milliseconds() - 1000 // just outside the grace window
		if err := st.SoftDeleteSession(ctx, session.ID, deletedAt); err != nil {
			t.Fatalf("soft delete: %v", err)
		}

		if _, err := st.SweepTombstones(ctx, sweepGrace, frozenNow); err != nil {
			t.Fatalf("SweepTombstones: %v", err)
		}

		if _, err := st.GetSession(ctx, session.ID); err == nil {
			t.Fatal("row survived though it is outside the grace window measured against the injected `now` -- a real time.Now() leaked into the age comparison")
		}
		assertSweepLastRun(t, st, ctx, frozenNow)
	})
}

// assertSweepLastRun checks the persisted throttle stamp is exactly the
// injected `now`, catching a real-clock leak on the write half of the
// sweep as well as on its age comparison.
func assertSweepLastRun(t *testing.T, st *Store, ctx context.Context, want int64) {
	t.Helper()
	raw, err := st.getUIState(ctx, tombstoneSweepLastRunKey, "0")
	if err != nil {
		t.Fatal(err)
	}
	if raw != strconv.FormatInt(want, 10) {
		t.Fatalf("ui_state %s = %q, want the injected now %d (a wall-clock value here means time.Now() leaked into the throttle write)", tombstoneSweepLastRunKey, raw, want)
	}
}

// TestSweepTombstonesRejectsInvalidArgs guards SweepTombstones' own input
// validation, mirroring EnforceEventRetention's equivalent guard.
func TestSweepTombstonesRejectsInvalidArgs(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	if _, err := st.SweepTombstones(ctx, -time.Second, 1_700_000_000_000); err == nil {
		t.Fatal("SweepTombstones(deleteGrace<0) = nil error, want an error")
	}
	if _, err := st.SweepTombstones(ctx, sweepGrace, 0); err == nil {
		t.Fatal("SweepTombstones(now=0) = nil error, want an error")
	}
}

// TestSweepTombstonesReapsOneBoundedBatchPerCall pins two things together
// (review finding 3, R79): the store-open, pre-first-frame call is STILL
// bounded to exactly one batch (whatever the backlog's real size), but the
// backlog as a whole no longer survives that same open/startup cycle --
// task 202 replaces the old assertion that a >2-batch backlog needed
// separate real-hour-spaced passes to finish (which pinned the very defect
// review finding 3 flagged) with the store's own reported remainder
// driving DrainExpiredTombstones, the production continuation
// cmd/deck's tick caller uses, to completion at essentially the SAME `now`
// -- no hour jumps manufactured by this test's own clock.
func TestSweepTombstonesReapsOneBoundedBatchPerCall(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()

	const now int64 = 1_700_000_000_000
	cutoff := now - sweepGrace.Milliseconds()

	// More than 2x tombstoneSweepBatchRows (200), so the backlog needs three
	// batches to drain: 200, 200, 50.
	const expiredCount = 450
	// deleted_at descends with i, so the HIGHEST i is the oldest tombstone
	// and must be reaped first.
	for i := 0; i < expiredCount; i++ {
		id := "sweep-batch-expired-" + strconv.Itoa(i)
		s := createTombstoneTestSession(t, st, ctx, id)
		if err := st.SoftDeleteSession(ctx, s.ID, cutoff-int64(i)-1); err != nil {
			t.Fatalf("soft delete %q: %v", id, err)
		}
	}
	// Fresh control row stays inside the grace window measured against `now`
	// itself: every call below (the first bounded batch AND the drain that
	// follows it) is driven at this SAME now, so one cutoff protects it
	// throughout the whole open/startup cycle, not just the first pass.
	fresh := createTombstoneTestSession(t, st, ctx, "sweep-batch-fresh")
	if err := st.SoftDeleteSession(ctx, fresh.ID, cutoff+1); err != nil {
		t.Fatalf("soft delete fresh: %v", err)
	}

	countExpiredTombstones := func() int {
		t.Helper()
		var n int
		if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE deleted_at != 0 AND id != ?`, fresh.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// The pre-first-frame call (deck's store-open call site): exactly one
	// bounded batch, no more -- and it must say so via its own reported
	// remainder, not leave the caller to guess from the row count.
	more, err := st.SweepTombstones(ctx, sweepGrace, now)
	if err != nil {
		t.Fatalf("first (frame-blocking) SweepTombstones: %v", err)
	}
	if !more {
		t.Fatal("first SweepTombstones reported no remainder, want true: a 450-row backlog is bigger than one 200-row batch")
	}
	if got, want := countExpiredTombstones(), expiredCount-tombstoneSweepBatchRows; got != want {
		t.Fatalf("expired tombstones after the frame-blocking call = %d, want %d (one call must reap exactly one bounded batch of %d and return, never the whole %d-row backlog)", got, want, tombstoneSweepBatchRows, expiredCount)
	}
	// Oldest first: the highest indices went, the lowest ones stayed.
	if _, err := st.GetSession(ctx, "sweep-batch-expired-"+strconv.Itoa(expiredCount-1)); err == nil {
		t.Fatal("the oldest tombstone survived the first pass, want oldest-first reaping")
	}
	if _, err := st.GetSession(ctx, "sweep-batch-expired-0"); err != nil {
		t.Fatalf("the newest expired tombstone was reaped in the first pass, want it left for a later one: %v", err)
	}

	// The startup/tick caller's own continuation: DrainExpiredTombstones
	// keeps calling SweepTombstones -- driven purely by ITS reported
	// remainder, never by a batch count this test computed -- until the
	// whole backlog above is gone, all within the SAME open/startup cycle
	// (same `now`, no hour jumps).
	if err := st.DrainExpiredTombstones(ctx, sweepGrace, now); err != nil {
		t.Fatalf("DrainExpiredTombstones: %v", err)
	}
	if got := countExpiredTombstones(); got != 0 {
		t.Fatalf("expired tombstones after DrainExpiredTombstones = %d, want 0 (the whole backlog must drain within the same open/startup cycle, review finding 3/R79)", got)
	}
	if _, err := st.GetSession(ctx, fresh.ID); err != nil {
		t.Fatalf("fresh tombstone was swept up, want it left alone: %v", err)
	}
}
