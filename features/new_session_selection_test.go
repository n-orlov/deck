package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerNewSessionSelectionSteps backs features/new_session_selection.feature
// (task 302, requirement 52): the one step here that no earlier task needed --
// a NON-POLLING check ("waits out a whole reconcile cadence, then reads the
// frame exactly once") that a selection survives a subsequent reconcile tick,
// mirroring clientScreenStillContainsAfterReconcileInterval's own reasoning
// but keyed on the "> name" selection marker clientHasSessionSelected already
// establishes, rather than on arbitrary screen text.
func registerNewSessionSelectionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^after one configured reconcile interval deck client "([^"]+)" has session "([^"]+)" selected$`, clientStillHasSessionSelectedAfterReconcileInterval)
}

// clientStillHasSessionSelectedAfterReconcileInterval proves requirement
// 52's one-shot promise: a user-driven selection made after a create's
// auto-selection intent already fired must not be re-stolen by a LATER
// sessionsLoaded (the next scheduled reconcile pass). Deliberately not a
// polling assertion -- it waits out a complete cadence first, then inspects
// the frame exactly once, so a standing "always select the newest" bug that
// only re-steals the selection on the tick AFTER the one already observed
// cannot hide from it.
func clientStillHasSessionSelectedAfterReconcileInterval(ctx context.Context, clientName, sessionName string) error {
	timer := time.NewTimer(scenarioReconcileInterval + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	marker := "> " + sessionName
	frame := client.Frame(false)
	if !strings.Contains(frame, marker) {
		return fmt.Errorf("deck client %q does not have session %q selected after a reconcile interval:\n%s", clientName, sessionName, frame)
	}
	return nil
}
