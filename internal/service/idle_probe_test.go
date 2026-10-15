package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R314: an idle row is probe-eligible after 4 x stale_after (a sparse check for
// a turn begun without a hook). This file is the covering test for the retired
// "idle is never probe-eligible" assertion in
// TestProbeEligibilityIncludesOnlySignalStatesAtWallClockBoundary.

func TestIdleRowEligibleOnlyAfterFourTimesStaleAfter(t *testing.T) {
	const stale = 45 * time.Second
	now := time.UnixMilli(1_000_000)
	cadence := 4 * stale.Milliseconds()
	row := store.Session{Status: "idle", StatusAt: now.UnixMilli() - cadence}
	if !probeEligible(row, now, stale) {
		t.Fatal("idle row was not eligible exactly at 4 x stale_after")
	}
	row.StatusAt++
	if probeEligible(row, now, stale) {
		t.Fatal("idle row was eligible one millisecond before 4 x stale_after")
	}
	row.StatusAt = now.UnixMilli() - stale.Milliseconds()
	if probeEligible(row, now, stale) {
		t.Fatal("idle row was eligible at 1 x stale_after")
	}
	for _, status := range []string{"error", "stopped"} {
		if probeEligible(store.Session{Status: status, StatusAt: 1}, now, stale) {
			t.Fatalf("%s row must never be probe-eligible", status)
		}
	}
}

// idleProbeFixture creates a claude row at `status` whose pane prints the named
// golden (or the literal text when golden is empty).
func idleProbeFixture(t *testing.T, id, status, golden, literal string, ageMs int64) (Service, *store.Store, store.Session) {
	t.Helper()
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "idle-probe-"+id)
	now := svc.Clock.Now().UnixMilli()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000003" + id, Name: "idle probe " + id, CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: status, StatusSource: "hook",
		StatusAt: now - ageMs, CreatedAt: now - ageMs,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := literal
	if golden != "" {
		raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "claude", golden))
		if err != nil {
			t.Fatal(err)
		}
		text = string(raw)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `printf '%s' "$1"; sleep 30`, "probe-fixture", text},
	}); err != nil {
		t.Fatal(err)
	}
	return svc, db, session
}

func probeEventCount(t *testing.T, db *store.Store, id string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind LIKE 'probe.%'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIdleRowIsNotProbedBeforeCadenceAndIsProbedAfter(t *testing.T) {
	const stale = 45 * time.Second
	// 3 x stale_after old: the pane shows the working indicator, but the row is not yet due.
	svc, db, session := idleProbeFixture(t, "01", "idle", "running.txt", "", 3*stale.Milliseconds())
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "idle" || got.StatusSource != "hook" || probeEventCount(t, db, session.ID) != 0 {
		t.Fatalf("idle row probed before the cadence: %#v", got)
	}
	// 4 x stale_after old: due. Same pane, same row, so only the age differs.
	svc, db, session = idleProbeFixture(t, "02", "idle", "running.txt", "", 4*stale.Milliseconds())
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" {
		t.Fatalf("idle row at the cadence was not probed: %#v", got)
	}
}

func TestProbeSeeingWorkingIndicatorMovesIdleRowToRunningWithSourceProbe(t *testing.T) {
	const stale = 45 * time.Second
	svc, db, session := idleProbeFixture(t, "03", "idle", "running.txt", "", 5*stale.Milliseconds())
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "probe" {
		t.Fatalf("idle row after a working-indicator probe = status %q source %q", got.Status, got.StatusSource)
	}
	if n := probeEventCount(t, db, session.ID); n != 1 {
		t.Fatalf("probe events = %d, want 1", n)
	}
}

func TestProbeMatchingNothingLeavesIdleRowIdle(t *testing.T) {
	const stale = 45 * time.Second
	svc, db, session := idleProbeFixture(t, "04", "idle", "", "nothing a rule matches\n", 5*stale.Milliseconds())
	before, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "idle" || got.StatusSource != before.StatusSource || got.StatusAt != before.StatusAt || probeEventCount(t, db, session.ID) != 0 {
		t.Fatalf("a probe that matched nothing changed the idle row: before %#v after %#v", before, got)
	}
}

func TestErrorRowIsNotProbedEvenWithWorkingIndicator(t *testing.T) {
	const stale = 45 * time.Second
	svc, db, session := idleProbeFixture(t, "05", "error", "running.txt", "", 100*stale.Milliseconds())
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" || probeEventCount(t, db, session.ID) != 0 {
		t.Fatalf("error row was probed: %#v", got)
	}
}
