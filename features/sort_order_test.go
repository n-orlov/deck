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
	sc.Step(`^the state database session "([^"]+)" has status_at ([0-9]+) seconds ago$`, setSessionStatusAtSecondsAgo)
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

// setSessionStatusAtSecondsAgo poses sort_order = activity's own comparator
// key (StatusAt) at an exact age without touching status or status_source,
// the way setSessionStatusSecondsAgo (features/attention_sort_test.go) does
// for both together. task 804's sort_order.feature error rows reach "error"
// via a genuine nonzero pane exit (features/crash_test.go's
// shellSessionExitsWithNonzeroStatus) instead of a raw status write, so the
// row carries an actual pane-exit verdict and a genuine tmux.pane_dead
// crash-collection event -- the fixture's error tier needs a real dead pane
// either way, since a raw "error" write only risks internal/service.
// reconcile's repairTerminalRowWithLivePane if it also carries a pane-exit
// or tmux/user-sourced verdict (a bare hook/probe-sourced error is left
// alone, finding F40, task 901), and a raw write here has neither.
// That genuine route settles status_at at whatever real wall-clock moment
// reconcile's crash collection actually runs, which the activity scenario's
// engineered relative ages cannot tolerate; by the time this step is ever
// used the row's pane is already dead and collected (crash collection kills
// it as part of recording the crash), so this raw, status-preserving write
// can never race that crash collection the way a raw status write would.
func setSessionStatusAtSecondsAgo(ctx context.Context, name string, secondsAgo int) error {
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
	result, err := db.ExecContext(ctx, `UPDATE sessions SET status_at = ? WHERE name = ?`, at, name)
	if err != nil {
		return fmt.Errorf("set session %q status_at %ds ago: %w", name, secondsAgo, err)
	}
	if err := requireOneRowAffected(result, "set session %q status_at %ds ago", name, secondsAgo); err != nil {
		return err
	}
	return nil
}
