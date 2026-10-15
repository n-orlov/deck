package store

import (
	"context"
	"testing"
	"time"
)

// R315: sessions.probe_miss_streak counts consecutive probe total misses. It is
// a column written by UPDATE, never an event, and Session.Unverified derives
// the display marker from it. These tests cover the store half of the six
// cases (2 misses, 3 misses, a probe hit clears, a hook verdict clears); the
// status text and sort position cases live in internal/tui.

const unverifiedStale = 45 * time.Second

func streakRow(t *testing.T, db *Store) Session {
	t.Helper()
	row, err := db.GetSession(context.Background(), "row")
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func seedRunning(t *testing.T) *Store {
	t.Helper()
	db := openRepeatStore(t)
	if _, err := db.CreateSession(context.Background(), CreateSessionInput{
		ID: "row", Name: "row", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

func missTimes(t *testing.T, db *Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := db.RecordProbeMiss(context.Background(), "row", int64(2000+i)); err != nil {
			t.Fatal(err)
		}
	}
}

func eventCount(t *testing.T, db *Store) int {
	t.Helper()
	var n int
	if err := db.db.QueryRow(`SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// farLater is past 3 x stale_after from the seeded status_at.
var farLater = time.UnixMilli(1000 + 3*unverifiedStale.Milliseconds())

func TestTwoProbeMissesLeaveARunningRowVerified(t *testing.T) {
	db := seedRunning(t)
	missTimes(t, db, 2)
	row := streakRow(t, db)
	if row.ProbeMissStreak != 2 {
		t.Fatalf("streak = %d, want 2", row.ProbeMissStreak)
	}
	if row.Unverified(farLater, unverifiedStale) {
		t.Fatal("two misses marked the row unverified")
	}
}

func TestThreeProbeMissesMarkARunningRowUnverifiedOnlyAfterThreeStaleWindows(t *testing.T) {
	db := seedRunning(t)
	events := eventCount(t, db)
	missTimes(t, db, 3)
	row := streakRow(t, db)
	if !row.Unverified(farLater, unverifiedStale) {
		t.Fatal("three misses and 3 x stale_after of silence did not mark the row unverified")
	}
	if row.Unverified(farLater.Add(-time.Millisecond), unverifiedStale) {
		t.Fatal("row was unverified one millisecond before 3 x stale_after")
	}
	if row.Unverified(farLater, 0) {
		t.Fatal("row was unverified with probing disabled (stale_after 0)")
	}
	if got := eventCount(t, db); got != events {
		t.Fatalf("probe misses wrote %d events, want none (a column, never an event)", got-events)
	}
	row.Status = "waiting"
	if row.Unverified(farLater, unverifiedStale) {
		t.Fatal("a waiting row was marked unverified; only running rows are")
	}
}

func TestProbeHitClearsTheMissStreak(t *testing.T) {
	for _, tc := range []struct{ name, status string }{
		{"repeat of running", "running"},
		{"changes the status", "idle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := seedRunning(t)
			missTimes(t, db, 3)
			if err := db.UpdateSessionStatus(context.Background(), StatusUpdateInput{
				SessionID: "row", Status: tc.status, Reason: "working indicator", Source: "probe",
				At: 5000, StaleAfter: unverifiedStale.Milliseconds(), EventKind: "probe." + tc.status,
			}); err != nil {
				t.Fatal(err)
			}
			row := streakRow(t, db)
			if row.ProbeMissStreak != 0 || row.Unverified(farLater.Add(time.Hour), unverifiedStale) {
				t.Fatalf("streak = %d after a probe hit, want 0 and not unverified", row.ProbeMissStreak)
			}
		})
	}
}

func TestHookVerdictClearsTheMissStreak(t *testing.T) {
	db := seedRunning(t)
	missTimes(t, db, 3)
	if err := db.UpdateSessionStatus(context.Background(), StatusUpdateInput{
		SessionID: "row", Status: "running", Reason: "UserPromptSubmit", Source: "hook",
		At: 6000, EventKind: "hook.UserPromptSubmit",
	}); err != nil {
		t.Fatal(err)
	}
	row := streakRow(t, db)
	if row.ProbeMissStreak != 0 || row.Unverified(farLater.Add(time.Hour), unverifiedStale) {
		t.Fatalf("streak = %d after a hook verdict, want 0 and not unverified", row.ProbeMissStreak)
	}
}

func TestStreakCountsConsecutiveMissesAcrossAHit(t *testing.T) {
	db := seedRunning(t)
	missTimes(t, db, 2)
	if err := db.UpdateSessionStatus(context.Background(), StatusUpdateInput{
		SessionID: "row", Status: "running", Reason: "working indicator", Source: "probe",
		At: 5000, StaleAfter: unverifiedStale.Milliseconds(), EventKind: "probe.running",
	}); err != nil {
		t.Fatal(err)
	}
	missTimes(t, db, 2)
	if row := streakRow(t, db); row.ProbeMissStreak != 2 || row.Unverified(farLater.Add(time.Hour), unverifiedStale) {
		t.Fatalf("streak = %d, want 2 (misses are consecutive)", row.ProbeMissStreak)
	}
}
