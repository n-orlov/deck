package features

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// TestStatusRecoveryStoppedFromHookSurvivesForcedSelfHeal is R170's own
// regression guard for status_recovery.feature:23 ("r on a terminal row
// whose tmux session already exists never reaches an error, however fast
// Reconcile's self-heal lands"). It runs that exact scenario -- the live
// .feature file on disk, not a copy -- through godog a second time, with
// one extra hook spliced onto the shared step registry: immediately after
// the step that fires fake Claude's SessionEnd hook, it sleeps a full
// configured reconcile interval before letting the NEXT step run.
// Reconcile ticks on its own cadence the whole time deck client "A" is
// alive, independently of the hook, so sleeping that long guarantees at
// least one more pass has already run by the time the scenario's very next
// step reads anything -- landing the self-heal before the read on every
// single invocation, deterministically, rather than leaving the ordering to
// whatever the host happened to schedule (the M8 inventory entry in
// /run/ralphd/artifacts/inventory/inventory.md shows CI losing that race
// only some of the time, which is what let it hide for a while).
//
// Before task 004 (R170), the scenario's own next step was a one-shot
// SELECT of the live sessions row demanding status "stopped"/"hook": forcing
// self-heal to land first makes Reconcile's own write (status "starting",
// source "tmux") the only thing a bare SELECT can ever observe at that
// point, so that old step failed every time under this forcing, not just
// occasionally. The fixed step instead reads the append-only events table
// for the hook's own "session_end" record, which Reconcile's pass neither
// touches nor can race: forcing self-heal ahead of the read changes nothing
// about whether it is still there. See artifacts/004/unfixed-regression.txt
// for this exact test, unmodified, run against an export of a192accf7d
// (whose copy of status_recovery.feature still has the old one-shot read):
// it fails there, and passes here.
func TestStatusRecoveryStoppedFromHookSurvivesForcedSelfHeal(t *testing.T) {
	const sessionEndFireStepText = `fires "SessionEnd" for itself using injected identity`

	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			initializeScenario(sc)
			sc.StepContext().After(func(ctx context.Context, step *godog.Step, _ godog.StepResultStatus, err error) (context.Context, error) {
				if strings.Contains(step.Text, sessionEndFireStepText) {
					// Deliberate: this is the forcing mechanism itself, not a
					// retry or a widened deadline on a product assertion --
					// see this test's own doc comment above.
					time.Sleep(scenarioReconcileInterval + 500*time.Millisecond)
				}
				return ctx, err
			})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"status_recovery.feature:23"},
			Tags:     defaultTags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("status_recovery.feature:23 failed when Reconcile's self-heal was forced to land before the scenario's next read; see output above")
	}
}
