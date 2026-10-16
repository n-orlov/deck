package service

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R303 / R316 / R317 / SPEC §7: a fresh hook verdict keeps precedence over
// every sampled source -- probes and the movement fallback -- including when
// the hook repeats the status the row already has. Freshness is the hook's own
// clock (sessions.hook_at), not status_at, which a repeat never moves.

func repeatHookAt(t *testing.T, db *store.Store, session store.Session, status string, at time.Time) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: status, Reason: "repeat", Source: "hook", At: at.UnixMilli(), EventKind: "repeat",
	}); err != nil {
		t.Fatal(err)
	}
}

// The movement fallback must not override a hook that just repeated idle,
// whatever the age of the idle transition itself.
func TestRepeatedHookIsFreshForMovement(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "copilot"} {
		t.Run(kind, func(t *testing.T) {
			r := newMovementRig(t, kind, 2*movementStale)
			now := r.svc.Clock.Now()
			repeatHookAt(t, r.db, r.row(), "idle", now.Add(-time.Second))
			if got := r.row(); got.StatusAt != now.Add(-2*movementStale).UnixMilli() {
				t.Fatalf("the repeat moved status_at to %d", got.StatusAt)
			}
			r.outputInTwoBuckets()
			if got := r.row(); got.Status != "idle" || got.StatusSource != "hook" {
				t.Fatalf("movement overrode a fresh repeated hook: status %q source %q", got.Status, got.StatusSource)
			}
		})
	}
}

// Not in the finding: the same repeat, but stale, lets movement through; and
// the repeat-era row still obeys R303 (status_at untouched).
func TestStaleRepeatedHookStillPermitsMovement(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "copilot"} {
		t.Run(kind, func(t *testing.T) {
			r := newMovementRig(t, kind, 3*movementStale)
			repeatHookAt(t, r.db, r.row(), "idle", r.svc.Clock.Now().Add(-2*movementStale))
			r.outputInTwoBuckets()
			if got := r.row(); got.Status != "running" || got.StatusSource != "movement" {
				t.Fatalf("a stale repeated hook blocked movement: status %q source %q", got.Status, got.StatusSource)
			}
		})
	}
}

// Not in the finding: a movement-sourced running row is taken back to idle
// only when no hook repeated recently; a fresh running repeat over a
// movement-set row also hands the row to the hook source.
func TestHookRepeatingRunningTakesTheRowFromMovement(t *testing.T) {
	r := newMovementRig(t, "claude", 2*movementStale)
	r.outputInTwoBuckets()
	r.wantStatus("running", "stale hook lets movement in")
	repeatHookAt(t, r.db, r.row(), "running", r.svc.Clock.Now())
	r.tick(1040, 1020, "start\nmore\nmost", "ctx:1%") // quiet bucket passed
	if got := r.row(); got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("movement took back a hook-sourced running row: status %q source %q", got.Status, got.StatusSource)
	}
}

func TestProbeEligibilityReadsHookEvidenceNotStatusAt(t *testing.T) {
	const stale = 45 * time.Second
	now := time.Unix(10_000, 0)
	old := now.Add(-10 * stale).UnixMilli()
	for _, status := range []string{"starting", "running", "waiting", "idle"} {
		session := store.Session{Status: status, StatusSource: "hook", StatusAt: old, HookAt: now.Add(-time.Second).UnixMilli()}
		if probeEligible(session, now, stale) {
			t.Errorf("%s row whose hook repeated a second ago is probe-eligible", status)
		}
		session.HookAt = old
		if !probeEligible(session, now, stale) {
			t.Errorf("%s row with a genuinely stale hook is not probe-eligible", status)
		}
		// Hook evidence only counts for a hook-sourced verdict.
		session.StatusSource, session.HookAt = "probe", now.Add(-time.Second).UnixMilli()
		if !probeEligible(session, now, stale) {
			t.Errorf("%s probe-sourced row borrowed an old hook's freshness", status)
		}
	}
}

func TestMovementStaleAfterReadsHookEvidence(t *testing.T) {
	const stale = time.Minute
	now := time.Unix(10_000, 0)
	session := store.Session{Agent: "claude", StatusSource: "hook", StatusAt: 1, HookAt: now.Add(-time.Second).UnixMilli()}
	if _, vetoed := movementStaleAfter(session, "running", now, stale); !vetoed {
		t.Fatal("a hook repeated a second ago did not veto movement")
	}
	session.HookAt = 1
	if _, vetoed := movementStaleAfter(session, "running", now, stale); vetoed {
		t.Fatal("a stale hook vetoed movement")
	}
}

// A sampled pane sees idle right after a hook repeated running: the repeat is
// fresh, so the probe neither runs against it nor wins (the store guard
// holds for a stale read as well, see the store tests).
func TestRepeatedHookDefeatsProbe(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-repeat-fresh")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 3)
	repeatHookAt(t, db, row, "running", svc.Clock.Now().Add(-time.Second))
	if got := probeCopilotRow(t, svc, db, row); got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("probe replaced a just-repeated hook: status %q source %q", got.Status, got.StatusSource)
	}
}

// Not in the finding: the same repeat dated a full window ago is stale, and
// the sampled idle verdict takes over.
func TestStaleRepeatedHookIsProbed(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-repeat-stale")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 4)
	repeatHookAt(t, db, row, "running", svc.Clock.Now().Add(-config.DefaultStaleAfter))
	if got := probeCopilotRow(t, svc, db, row); got.Status != "idle" || got.StatusSource != "probe" {
		t.Fatalf("a stale repeated hook was not probed: status %q source %q", got.Status, got.StatusSource)
	}
}

// The probe that raced ahead of a hook repeat: it read the row before the
// repeat landed, so only the atomic store guard can refuse it.
func TestStaleProbeReadCannotOverrideAHookRepeatedMeanwhile(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-repeat-race")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 5)
	repeatHookAt(t, db, row, "running", svc.Clock.Now())
	// row is the pre-repeat read; recordProbe is what the probing pass calls.
	if err := svc.recordProbe(context.Background(), row, "idle", "ready", config.DefaultStaleAfter); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("a probe from before the repeat won: status %q source %q", got.Status, got.StatusSource)
	}
}
