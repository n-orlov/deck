package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerTerminalRepairFieldRouteSteps backs
// features/terminal_repair_field_route.feature (task 002, requirement 76):
// the field's own reconcile pass, not a keypress, repairs a terminal row
// whose pane survived a hook write. The one step registered here polls the
// durable row instead of taking a single reading, because the hook's own
// "stopped" write and Reconcile's later correction race the harness's own
// query -- a single SELECT can observe either verdict depending on exactly
// when it lands, and the row's OWN "starting" word already appears on
// screen before the hook ever fires (a fresh agent row starts there), so a
// screen-text match for "starting" would pass on stale, pre-hook content
// without ever observing the repair. Polling the database directly for the
// repaired (status, source, killed_by_user) tuple is the one check immune to
// both races.
func registerTerminalRepairFieldRouteSteps(sc *godog.ScenarioContext) {
	sc.Step(`^within one configured reconcile interval the state database session "([^"]+)" is "([^"]+)" from "([^"]+)" with killed_by_user=([01])$`, databaseSessionTerminalFieldsWithinReconcileInterval)
}

func databaseSessionTerminalFieldsWithinReconcileInterval(ctx context.Context, name, wantStatus, wantSource string, wantKilled int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(scenarioReconcileInterval + 3*time.Second)
	var status, source string
	var killed int
	for {
		if err := db.QueryRowContext(ctx, `SELECT status, status_source, killed_by_user FROM sessions WHERE name = ?`, name).Scan(&status, &source, &killed); err != nil {
			return fmt.Errorf("observe terminal fields for session %q: %w", name, err)
		}
		if status == wantStatus && source == wantSource && killed == wantKilled {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q terminal fields never reached status %q, source %q, killed_by_user=%d within one reconcile interval; last observed status %q, source %q, killed_by_user=%d", name, wantStatus, wantSource, wantKilled, status, source, killed)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
