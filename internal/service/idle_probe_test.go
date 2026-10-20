package service

import (
	"context"
	"github.com/n-orlov/deck/internal/config"
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

// R314 cadence: the sparse idle check repeats every 4 x stale_after, measured
// from the last sample, not from the (unchanged) verdict.

func TestIdleProbeEligibilityIsAnchoredOnTheLastSample(t *testing.T) {
	const stale = 45 * time.Second
	cadence := 4 * stale.Milliseconds()
	now := time.UnixMilli(10_000_000)
	row := store.Session{Status: "idle", StatusAt: 1, LastProbeAt: now.UnixMilli() - cadence + 1}
	if probeEligible(row, now, stale) {
		t.Fatal("idle row sampled one millisecond ago-of-cadence was due again")
	}
	row.LastProbeAt = now.UnixMilli() - cadence
	if !probeEligible(row, now, stale) {
		t.Fatal("idle row was not due exactly one cadence after its last sample")
	}
	// A sample older than the verdict never pulls the anchor backwards.
	row = store.Session{Status: "idle", StatusAt: now.UnixMilli() - 10, LastProbeAt: 5}
	if probeEligible(row, now, stale) {
		t.Fatal("a stale sample made a fresh idle verdict due")
	}
	// An error row stays excluded however old its last sample is.
	if probeEligible(store.Session{Status: "error", StatusAt: 1, LastProbeAt: 1}, now, stale) {
		t.Fatal("error row became eligible")
	}
}

// advanceAndReconcile steps the frozen clock one step and runs a probing pass.
func advanceAndReconcile(t *testing.T, svc Service) {
	const stale = 45 * time.Second
	t.Helper()
	svc.Clock.Advance()
	if err := svc.ReconcileWithProbes(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
}

func steppedClock(t *testing.T, svc *Service, step time.Duration) {
	t.Helper()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", step.String())
	if err != nil {
		t.Fatal(err)
	}
	svc.Clock = clock
}

func TestMatchingIdleVerdictIsResampledOnlyAtTheNextCadence(t *testing.T) {
	const stale = 45 * time.Second
	for _, golden := range []string{"idle-draft.txt", "idle-scrolled.txt"} {
		t.Run(golden, func(t *testing.T) {
			svc, db, session := idleProbeFixture(t, "10", "idle", golden, "", 6*stale.Milliseconds())
			steppedClock(t, &svc, stale)
			ctx := context.Background()
			if err := svc.ReconcileWithProbes(ctx, stale); err != nil {
				t.Fatal(err)
			}
			if n := probeEventCount(t, db, session.ID); n != 1 {
				t.Fatalf("first due pass wrote %d probe events, want 1", n)
			}
			// 1x, 2x and 3x stale_after after the sample: not due, however old the verdict is.
			for step := 1; step <= 3; step++ {
				advanceAndReconcile(t, svc)
				if n := probeEventCount(t, db, session.ID); n != 1 {
					t.Fatalf("idle row re-probed %d x stale_after after its sample: %d probe events", step, n)
				}
			}
			advanceAndReconcile(t, svc) // 4x: due.
			if n := probeEventCount(t, db, session.ID); n != 2 {
				t.Fatalf("idle row not re-probed at 4 x stale_after after its sample: %d probe events", n)
			}
			got, err := db.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "idle" {
				t.Fatalf("status = %q, want idle", got.Status)
			}
		})
	}
}

func TestProbeMissIsResampledOnlyAtTheNextCadence(t *testing.T) {
	const stale = 45 * time.Second
	for _, text := range []string{"nothing a rule matches\n", "still nothing\nat all\n"} {
		svc, db, session := idleProbeFixture(t, "11", "idle", "", text, 6*stale.Milliseconds())
		steppedClock(t, &svc, stale)
		ctx := context.Background()
		if err := svc.ReconcileWithProbes(ctx, stale); err != nil {
			t.Fatal(err)
		}
		first, err := db.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if first.ProbeMissStreak != 1 || first.LastProbeAt == 0 {
			t.Fatalf("first due pass: streak %d last_probe_at %d", first.ProbeMissStreak, first.LastProbeAt)
		}
		for step := 1; step <= 3; step++ {
			advanceAndReconcile(t, svc)
			got, err := db.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.ProbeMissStreak != 1 || got.LastProbeAt != first.LastProbeAt {
				t.Fatalf("missed idle row re-sampled %d x stale_after later: streak %d last_probe_at %d", step, got.ProbeMissStreak, got.LastProbeAt)
			}
		}
		advanceAndReconcile(t, svc)
		got, err := db.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ProbeMissStreak != 2 || got.Status != "idle" || got.StatusAt != first.StatusAt {
			t.Fatalf("missed idle row not re-sampled at the cadence: %#v", got)
		}
		if n := probeEventCount(t, db, session.ID); n != 0 {
			t.Fatalf("a miss wrote %d probe events", n)
		}
	}
}

func TestIdleRowMissedOnceIsPromotedAtTheNextCadenceWhenItIsWorking(t *testing.T) {
	const stale = 45 * time.Second
	cwd := t.TempDir()
	trigger := filepath.Join(cwd, "go")
	svc, db, _, _ := newAgentTestService(t, nil, "idle-probe-12")
	steppedClock(t, &svc, stale)
	ctx := context.Background()
	now := svc.Clock.Now().UnixMilli()
	session, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000312", Name: "idle probe 12", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "idle", StatusSource: "hook",
		StatusAt: now - 6*stale.Milliseconds(), CreatedAt: now - 6*stale.Milliseconds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "claude", "running.txt"))
	if err != nil {
		t.Fatal(err)
	}
	script := `printf 'nothing yet\n'; while [ ! -e "$1" ]; do sleep 0.05; done; printf '%s' "$2"; sleep 30`
	if _, err := svc.TMux.Create(ctx, tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", script, "probe-fixture", trigger, string(raw)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileWithProbes(ctx, stale); err != nil { // miss
		t.Fatal(err)
	}
	if got, _ := db.GetSession(ctx, session.ID); got.Status != "idle" || got.ProbeMissStreak != 1 {
		t.Fatalf("expected one miss on an idle row: %#v", got)
	}
	if err := os.WriteFile(trigger, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let the pane print the working indicator
	for step := 1; step <= 3; step++ {
		advanceAndReconcile(t, svc)
		if got, _ := db.GetSession(ctx, session.ID); got.Status != "idle" {
			t.Fatalf("working idle row promoted %d x stale_after after its last sample", step)
		}
	}
	advanceAndReconcile(t, svc)
	got, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "probe" {
		t.Fatalf("idle row not promoted at the next cadence: status %q source %q", got.Status, got.StatusSource)
	}
}

func TestFreshHookEvidenceDefersTheIdleSampleByAWholeCadence(t *testing.T) {
	const stale = 45 * time.Second
	svc, db, session := idleProbeFixture(t, "13", "idle", "running.txt", "", 10*stale.Milliseconds())
	ctx := context.Background()
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "idle", Reason: "hook repeat", Source: "hook",
		At: svc.Clock.Now().UnixMilli() - 3*stale.Milliseconds(), EventKind: "hook.idle",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileWithProbes(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetSession(ctx, session.ID); got.Status != "idle" || probeEventCount(t, db, session.ID) != 0 {
		t.Fatalf("idle row with a hook repeat 3 x stale_after ago was probed: %#v", got)
	}
}
