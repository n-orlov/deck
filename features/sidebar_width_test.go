package features

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/cucumber/godog"
)

// registerSidebarWidthSteps covers requirement 7: a step sets sidebar_width
// for a scenario and another reads it back, proving the round trip actually
// persists rather than a step that writes bytes no one reads. deck itself
// does not yet consume ui_state (that lands with tasks 10/11/15/16), so
// these steps talk to state.db directly, using the exact ui_state shape
// SPEC.md §11.2 documents (key TEXT PRIMARY KEY, value TEXT NOT NULL). They
// deliberately create the table with IF NOT EXISTS rather than assuming a
// migration has run, so the step works whether it runs before or after
// task 10 adds ui_state to deck's own schema.
func registerSidebarWidthSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario's sidebar_width is set to ([0-9]+)$`, sidebarWidthIsSetTo)
	sc.Step(`^the scenario's persisted sidebar_width reads back as ([0-9]+)$`, sidebarWidthReadsBackAs)
	sc.Step(`^the scenario's persisted sidebar_width is unset$`, sidebarWidthIsUnset)
}

// ensureUIStateTable opens the scenario's state.db and makes sure ui_state
// exists, matching SPEC.md §11.2's schema exactly. It requires state.db to
// already exist (created by a deck client that has already started), since
// creating it from scratch here would mean duplicating deck's own schema
// creation and risking drift.
func ensureUIStateTable(h *ScenarioHarness) (*sql.DB, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ui_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure ui_state table: %w", err)
	}
	return db, nil
}

func sidebarWidthIsSetTo(ctx context.Context, width int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := ensureUIStateTable(h)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO ui_state (key, value) VALUES ('sidebar_width', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		fmt.Sprintf("%d", width)); err != nil {
		return fmt.Errorf("set sidebar_width to %d: %w", width, err)
	}
	return nil
}

func sidebarWidthReadsBackAs(ctx context.Context, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := ensureUIStateTable(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT value FROM ui_state WHERE key = 'sidebar_width'`).Scan(&got); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sidebar_width is unset, want %d", want)
		}
		return fmt.Errorf("read back sidebar_width: %w", err)
	}
	if got != fmt.Sprintf("%d", want) {
		return fmt.Errorf("sidebar_width read back as %q, want %d", got, want)
	}
	return nil
}

func sidebarWidthIsUnset(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := ensureUIStateTable(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	err = db.QueryRowContext(ctx, `SELECT value FROM ui_state WHERE key = 'sidebar_width'`).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check sidebar_width unset: %w", err)
	}
	return fmt.Errorf("sidebar_width already set to %q, want unset", got)
}
