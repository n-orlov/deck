package features

import (
	"testing"
	"time"
)

// TestReconcileIntervalPollDeadlineRaceIndicator drives
// reconcileIntervalPollDeadline directly with an injected race indicator,
// proving R164's requirement that the widened deadline is applied only on
// a race build and that a normal build's deadline is unchanged from the
// fixed scenarioReconcileInterval+250ms=500ms bound every "within one
// configured reconcile interval ..." step used before this task.
func TestReconcileIntervalPollDeadlineRaceIndicator(t *testing.T) {
	const interval = scenarioReconcileInterval

	// Normal build (raceBuild=false): the exact pre-existing bound.
	if got, want := reconcileIntervalPollDeadline(interval, false), interval+250*time.Millisecond; got != want {
		t.Fatalf("normal build: reconcileIntervalPollDeadline(%s, false) = %s, want %s", interval, got, want)
	}

	// Race build (raceBuild=true): strictly wider than the normal-build
	// deadline, and wide enough to clear the 500ms bound that CI run
	// 36717507756 showed missed under -race.
	got := reconcileIntervalPollDeadline(interval, true)
	normal := reconcileIntervalPollDeadline(interval, false)
	if got <= normal {
		t.Fatalf("race build: reconcileIntervalPollDeadline(%s, true) = %s, want strictly wider than the normal-build deadline %s", interval, got, normal)
	}
	if got <= 500*time.Millisecond {
		t.Fatalf("race build: reconcileIntervalPollDeadline(%s, true) = %s, want more than the 500ms bound that missed under -race on CI (run 36717507756)", interval, got)
	}
}
