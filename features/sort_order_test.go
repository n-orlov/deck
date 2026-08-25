package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerSortOrderSteps backs features/sort_order.feature (task 307,
// requirement R53): the one harness addition this feature needs beyond
// features/attention_sort_test.go's existing direct-state.db steps -- a way
// to pose a session's sessions.created_at at an exact age, mirroring
// setSessionStatusSecondsAgo's own reasoning (the create modal gives every
// session created within one scenario the same real-clock CreatedAt
// ordering as their creation sequence, which is both slower to arrange and
// not exact enough to guarantee four DIFFERENT row sequences across all
// four sort orders on one shared fixture -- this step lets the scenario
// pin created_at independently of both creation order and the status/
// status_at fixture setSessionStatusSecondsAgo already poses).
//
// Unlike setSessionStatusSecondsAgo, this step never touches status or
// status_at, so it cannot race internal/service.reconcile's own status
// promotion the way a status write can -- created_at is written once at
// creation and reconcile never revisits it.
func registerSortOrderSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the state database session "([^"]+)" has created_at ([0-9]+) seconds ago$`, setSessionCreatedAtSecondsAgo)
}

func setSessionCreatedAtSecondsAgo(ctx context.Context, name string, secondsAgo int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	at := time.Now().Add(-time.Duration(secondsAgo) * time.Second).UnixMilli()
	result, err := db.ExecContext(ctx, `UPDATE sessions SET created_at = ? WHERE name = ?`, at, name)
	if err != nil {
		return fmt.Errorf("set session %q created_at %ds ago: %w", name, secondsAgo, err)
	}
	if err := requireOneRowAffected(result, "set session %q created_at %ds ago", name, secondsAgo); err != nil {
		return err
	}
	return nil
}
