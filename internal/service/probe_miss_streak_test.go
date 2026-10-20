package service

import (
	"context"
	"testing"
	"time"
)

// R315: each reconcile pass whose probe matches no rule adds one to
// sessions.probe_miss_streak (a column, no event, status untouched); a pass whose
// probe matches resets it.
func TestReconcileProbeMissesCountAsAColumnAndAHitClearsThem(t *testing.T) {
	const stale = 45 * time.Second
	svc, db, session := idleProbeFixture(t, "15", "running", "", "nothing a rule matches\n", 5*stale.Milliseconds())
	ctx := context.Background()
	events := func() int {
		var n int
		if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, session.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := events()
	for want := 1; want <= 3; want++ {
		if err := svc.ReconcileWithProbes(ctx, stale); err != nil {
			t.Fatal(err)
		}
		got, err := db.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ProbeMissStreak != want || got.Status != "running" || got.StatusAt != session.StatusAt {
			t.Fatalf("after %d misses: streak %d status %q status_at %d", want, got.ProbeMissStreak, got.Status, got.StatusAt)
		}
	}
	if events() != before {
		t.Fatalf("misses wrote events: %d -> %d", before, events())
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET probe_miss_streak = 3 WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	// Same row, now showing the working indicator: the probe hit resets it.
	hit, hitDB, hitSession := idleProbeFixture(t, "16", "running", "running.txt", "", 5*stale.Milliseconds())
	if _, err := hitDB.DB().Exec(`UPDATE sessions SET probe_miss_streak = 3 WHERE id = ?`, hitSession.ID); err != nil {
		t.Fatal(err)
	}
	if err := hit.ReconcileWithProbes(ctx, stale); err != nil {
		t.Fatal(err)
	}
	got, err := hitDB.GetSession(ctx, hitSession.ID)
	if err != nil || got.ProbeMissStreak != 0 {
		t.Fatalf("probe hit left streak %d, %v", got.ProbeMissStreak, err)
	}
}
