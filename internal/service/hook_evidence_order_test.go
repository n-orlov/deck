package service

import (
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// R303 / R316 / R317 / SPEC §7: hook deliveries can arrive out of order. A
// delayed same-status repeat dated before a newer one must not age the newest
// hook evidence, so reconcile's eligibility (probeEligible and the movement
// fallback's pre-check) and the store guards keep a fresh hook in charge.

// Finding case (movement): a fresh idle repeat, then a delayed one dated two
// windows back; movement still loses to the fresh hook.
func TestDelayedRepeatKeepsHookFreshForMovement(t *testing.T) {
	r := newMovementRig(t, "claude", 3*movementStale)
	now := r.svc.Clock.Now()
	repeatHookAt(t, r.db, r.row(), "idle", now.Add(-time.Second))
	repeatHookAt(t, r.db, r.row(), "idle", now.Add(-2*movementStale))
	if got := r.row(); got.HookAt != now.Add(-time.Second).UnixMilli() {
		t.Fatalf("delayed repeat aged hook_at to %d", got.HookAt)
	}
	r.outputInTwoBuckets()
	if got := r.row(); got.Status != "idle" || got.StatusSource != "hook" {
		t.Fatalf("movement overrode a fresh hook after a delayed repeat: %q/%q", got.Status, got.StatusSource)
	}
}

// Finding case (probe): the same reorder over running; the probe loses.
func TestDelayedRepeatKeepsHookFreshForProbe(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-repeat-reordered")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 5)
	now := svc.Clock.Now()
	repeatHookAt(t, db, row, "running", now.Add(-time.Second))
	repeatHookAt(t, db, row, "running", now.Add(-2*config.DefaultStaleAfter))
	if got := probeCopilotRow(t, svc, db, row); got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("probe replaced a fresh hook after a delayed repeat: %q/%q", got.Status, got.StatusSource)
	}
}

// Not in the finding: eligibility read from the stored row after the reorder,
// for both a running row (probe) and an idle row (probe and movement), and a
// genuinely stale newest hook makes both eligible again.
func TestEligibilityAfterReorderedRepeatsReadsNewestHook(t *testing.T) {
	const stale = time.Second
	for _, status := range []string{"running", "idle"} {
		t.Run(status, func(t *testing.T) {
			r := newMovementRig(t, "claude", 20*stale)
			base := r.svc.Clock.Now()
			repeatHookAt(t, r.db, r.row(), status, base.Add(2*stale))
			repeatHookAt(t, r.db, r.row(), status, base)
			session := r.row()
			fresh := base.Add(2*stale + stale/2)
			idleStale := stale
			if status == "idle" {
				idleStale = stale / idleProbeFactor
			}
			if probeEligible(session, fresh, idleStale) {
				t.Errorf("%s row probe-eligible 500ms after its newest hook", status)
			}
			if _, vetoed := movementStaleAfter(session, "running", fresh, stale); !vetoed {
				t.Errorf("%s row: movement pre-check let a fresh hook be replaced", status)
			}
			late := base.Add(2*stale + stale)
			if !probeEligible(session, late, idleStale) {
				t.Errorf("%s row not probe-eligible once its newest hook is stale", status)
			}
			if _, vetoed := movementStaleAfter(session, "running", late, stale); vetoed {
				t.Errorf("%s row: a stale newest hook still vetoed movement", status)
			}
		})
	}
}
