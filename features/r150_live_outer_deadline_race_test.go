package features

import (
	"testing"
	"time"
)

// TestR150LiveOuterDeadlineRaceIndicator (R164 cure, cure-01-01) drives
// r150LiveOuterDeadline directly with an injected race indicator, pinning
// that a normal build's outer-context budget is exactly the historical
// `base` (unchanged product/test behavior) while a race build's budget is
// strictly wider by exactly failingReconcileWaits copies of the race-build
// reconcileIntervalPollDeadline -- wide enough that two sequential rejected
// clientRowContainsWithinReconcile calls (the base and rename_only_control
// cases in TestR150LiveGroupedSidebarRowSettleProvesActualStatus and
// TestR150LiveNamedRowSettleIgnoresAnotherSessionsName) can each burn their
// full race-widened inner deadline without the outer context expiring
// first.
func TestR150LiveOuterDeadlineRaceIndicator(t *testing.T) {
	const base = 15 * time.Second
	const failingReconcileWaits = r150FailingReconcileWaits

	// Normal build: unchanged from the historical fixed base budget.
	if got := r150LiveOuterDeadline(base, false); got != base {
		t.Fatalf("normal build: r150LiveOuterDeadline(%s, false) = %s, want %s (unchanged)", base, got, base)
	}

	// Race build: strictly wider than the normal-build budget, and exactly
	// base plus failingReconcileWaits full race-build reconcile deadlines.
	got := r150LiveOuterDeadline(base, true)
	normal := r150LiveOuterDeadline(base, false)
	if got <= normal {
		t.Fatalf("race build: r150LiveOuterDeadline(%s, true) = %s, want strictly wider than the normal-build budget %s", base, got, normal)
	}
	perWait := reconcileIntervalPollDeadline(true)
	want := base + time.Duration(failingReconcileWaits)*perWait
	if got != want {
		t.Fatalf("race build: r150LiveOuterDeadline(%s, true) = %s, want %s (base + %d x race reconcile deadline %s)", base, got, want, failingReconcileWaits, perWait)
	}

	// Sanity: the race-build per-wait deadline alone already exceeds this
	// function's own fixed 20s widening constant's neighbourhood, so the
	// two-wait outer budget must clear 40s -- the exact interaction task
	// 018 found un-covered (2 x 20.25s = 40.5s against a fixed 15s/30s
	// outer context).
	if perWait <= 20*time.Second {
		t.Fatalf("race build: reconcileIntervalPollDeadline(true) [interval %s] = %s, want more than 20s so the widening above is meaningful", scenarioReconcileInterval, perWait)
	}
}
