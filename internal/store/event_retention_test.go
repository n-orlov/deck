package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows is
// task 332's (R62, steer 3e-001 §6.4) primary retention proof. It seeds
// rows on BOTH sides of the retention boundary, plus the boundary row
// itself, at a scale (1,200 old rows against eventRetentionBatchRows=500)
// that forces EnforceEventRetention's single call to loop across more than
// one batch to finish the job. It then asserts BOTH halves together: every
// old row is gone AND every recent row (including the boundary row)
// survives -- a one-sided assertion (only checking old rows are gone) would
// also pass a retention pass that deleted everything, and a one-sided
// assertion the other way (only checking recent rows survive) would also
// pass a pass that deletes nothing.
//
// The boundary row's fate: EnforceEventRetention's cutoff comparison is
// `WHERE at < cutoff` (strict), so a row whose at EQUALS the cutoff falls
// INSIDE the retention window and is kept, not deleted. This test's
// boundary row is therefore asserted present, not absent.
func TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	const retentionDays = 7
	const now int64 = 1_700_000_000_000 // an arbitrary fixed "current time" in UnixMilli
	cutoff := now - int64(retentionDays)*24*time.Hour.Milliseconds()

	insert := func(kind string, at int64) {
		if _, err := st.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload) VALUES (NULL, ?, ?, 'test', 'x')`, at, kind); err != nil {
			t.Fatalf("seed %s at %d: %v", kind, at, err)
		}
	}

	// 1,200 old rows -- more than 2x eventRetentionBatchRows (500) so a
	// single EnforceEventRetention call must loop across at least three
	// batches to clear all of them, proving "bounded batches" doesn't mean
	// "only one batch no matter how much is due".
	const oldCount = 1200
	for i := 0; i < oldCount; i++ {
		insert("old", cutoff-int64(i)-1000)
	}
	// The boundary row itself: at == cutoff exactly, kept per the doc above.
	insert("boundary", cutoff)
	// 50 recent rows, strictly inside the retention window.
	const recentCount = 50
	for i := 0; i < recentCount; i++ {
		insert("recent", cutoff+int64(i)+1)
	}

	var totalBefore int
	if err := st.db.QueryRow(`SELECT count(*) FROM events`).Scan(&totalBefore); err != nil {
		t.Fatal(err)
	}
	if want := oldCount + 1 + recentCount; totalBefore != want {
		t.Fatalf("seeded %d events, want %d", totalBefore, want)
	}

	if err := st.EnforceEventRetention(ctx, retentionDays, now); err != nil {
		t.Fatalf("EnforceEventRetention: %v", err)
	}

	var oldRemaining int
	if err := st.db.QueryRow(`SELECT count(*) FROM events WHERE kind = 'old'`).Scan(&oldRemaining); err != nil {
		t.Fatal(err)
	}
	if oldRemaining != 0 {
		t.Errorf("old rows remaining after EnforceEventRetention = %d, want 0 (all %d rows strictly older than the cutoff must be gone)", oldRemaining, oldCount)
	}

	var boundaryRemaining int
	if err := st.db.QueryRow(`SELECT count(*) FROM events WHERE kind = 'boundary'`).Scan(&boundaryRemaining); err != nil {
		t.Fatal(err)
	}
	if boundaryRemaining != 1 {
		t.Errorf("boundary row (at == cutoff) remaining = %d, want 1 (kept: the comparison is strict, at < cutoff)", boundaryRemaining)
	}

	var recentRemaining int
	if err := st.db.QueryRow(`SELECT count(*) FROM events WHERE kind = 'recent'`).Scan(&recentRemaining); err != nil {
		t.Fatal(err)
	}
	if recentRemaining != recentCount {
		t.Errorf("recent rows remaining = %d, want %d (every row inside the retention window must survive)", recentRemaining, recentCount)
	}

	var totalAfter int
	if err := st.db.QueryRow(`SELECT count(*) FROM events`).Scan(&totalAfter); err != nil {
		t.Fatal(err)
	}
	if want := 1 + recentCount; totalAfter != want {
		t.Fatalf("events remaining after EnforceEventRetention = %d, want %d (boundary + recent only)", totalAfter, want)
	}
}

// TestEnforceEventRetentionThrottlesToAtMostOnceAnHour is the second half
// of task 332's contract: "on store open and thereafter at most once an
// hour". The first call always runs (there is no ui_state row yet); a
// second call minutes later, against a fresh batch of rows that would
// otherwise be due for deletion, must be a throttled no-op; a third call
// more than an hour after the first must run again and catch up.
func TestEnforceEventRetentionThrottlesToAtMostOnceAnHour(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	const retentionDays = 1
	const firstRun int64 = 1_700_000_000_000
	cutoffAtFirstRun := firstRun - int64(retentionDays)*24*time.Hour.Milliseconds()

	insertAt := func(at int64) {
		if _, err := st.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload) VALUES (NULL, ?, 'probe', 'test', 'x')`, at); err != nil {
			t.Fatal(err)
		}
	}

	// One row already due for deletion at the very first call.
	insertAt(cutoffAtFirstRun - 1)
	if err := st.EnforceEventRetention(ctx, retentionDays, firstRun); err != nil {
		t.Fatalf("first EnforceEventRetention: %v", err)
	}
	var afterFirst int
	if err := st.db.QueryRow(`SELECT count(*) FROM events`).Scan(&afterFirst); err != nil {
		t.Fatal(err)
	}
	if afterFirst != 0 {
		t.Fatalf("events after first (store-open) EnforceEventRetention call = %d, want 0", afterFirst)
	}

	// 10 minutes later: a fresh row already due for deletion is seeded, then
	// EnforceEventRetention is called again. The throttle must make this a
	// no-op: the row must still be there.
	secondRun := firstRun + 10*time.Minute.Milliseconds()
	cutoffAtSecondRun := secondRun - int64(retentionDays)*24*time.Hour.Milliseconds()
	insertAt(cutoffAtSecondRun - 1)
	if err := st.EnforceEventRetention(ctx, retentionDays, secondRun); err != nil {
		t.Fatalf("second (throttled) EnforceEventRetention: %v", err)
	}
	var afterSecond int
	if err := st.db.QueryRow(`SELECT count(*) FROM events`).Scan(&afterSecond); err != nil {
		t.Fatal(err)
	}
	if afterSecond != 1 {
		t.Fatalf("events 10 minutes after the store-open run = %d, want 1 (throttled: no deletion pass should have run again yet)", afterSecond)
	}

	// More than an hour after the FIRST run: the throttle lets a real pass
	// through again, and the row seeded above (already due since the
	// second call) is finally cleared.
	thirdRun := firstRun + 61*time.Minute.Milliseconds()
	if err := st.EnforceEventRetention(ctx, retentionDays, thirdRun); err != nil {
		t.Fatalf("third EnforceEventRetention: %v", err)
	}
	var afterThird int
	if err := st.db.QueryRow(`SELECT count(*) FROM events`).Scan(&afterThird); err != nil {
		t.Fatal(err)
	}
	if afterThird != 0 {
		t.Fatalf("events more than an hour after the store-open run = %d, want 0 (the throttle must let a real pass through again)", afterThird)
	}
}

// TestEnforceEventRetentionRejectsNonPositiveWindow guards the one
// deliberately-not-schema-enforced invariant this method depends on: the
// schema's own IntBounds.Min=1 keeps a saved config.toml value sane, but
// EnforceEventRetention is called directly by tests and by cmd/deck/main.go
// with a settings value, so it refuses a non-positive window itself rather
// than silently treating it as "retain nothing" or "retain forever".
func TestEnforceEventRetentionRejectsNonPositiveWindow(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	if err := st.EnforceEventRetention(ctx, 0, 1_700_000_000_000); err == nil {
		t.Fatal("EnforceEventRetention(retentionDays=0) = nil error, want an error")
	}
	if err := st.EnforceEventRetention(ctx, 30, 0); err == nil {
		t.Fatal("EnforceEventRetention(now=0) = nil error, want an error")
	}
}
