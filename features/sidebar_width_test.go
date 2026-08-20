package features

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
//
// registerSidebarWidthSteps also carries the step that closes a prior
// validation's gap: sidebar_width round-tripping through state.db alone
// proves nothing about deck's own rendering, so a second scenario
// (features/harness.feature) instead drives the running client's own
// `<`/`>` keys (task 015, which reads and writes m.sidebarWidth directly —
// state.db persistence is task 016) and asserts a previously-cropped
// session name becomes visible once the sidebar actually widens.
func registerSidebarWidthSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario's sidebar_width is set to ([0-9]+)$`, sidebarWidthIsSetTo)
	sc.Step(`^the scenario's persisted sidebar_width reads back as ([0-9]+)$`, sidebarWidthReadsBackAs)
	sc.Step(`^the scenario's persisted sidebar_width is unset$`, sidebarWidthIsUnset)
	sc.Step(`^deck client "([^"]+)" creates a long-named shell session "([^"]+)"$`, clientCreatesLongNamedShellSession)
	sc.Step(`^deck client "([^"]+)" presses ">" until "([^"]+)" is visible$`, clientWidensSidebarUntilVisible)
}

// clientWidensSidebarUntilVisible drives `>` (task 015) one keystroke at a
// time with a short pause between presses, rather than writing a burst of
// `>` bytes in one PTY write: a burst sent faster than deck's own Bubble
// Tea input loop drains it is observed to be silently dropped past the
// first byte (reproduced standalone: 40 `>` bytes in one write, or 10 rapid
// separate writes with no pause, both leave sidebarWidth exactly one column
// wider than before the burst, never anywhere close to the ceiling), so
// this paces the presses like a real held-down key instead. It stops as
// soon as want appears rather than assuming a fixed press count, since the
// exact number of columns needed depends on the terminal width the
// scenario happens to run at.
func clientWidensSidebarUntilVisible(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	const maxPresses = 60 // comfortably above any terminal width's sidebar ceiling
	for i := 0; i < maxPresses; i++ {
		if strings.Contains(client.Frame(false), want) {
			return nil
		}
		if err := client.Send(">"); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	if strings.Contains(client.Frame(false), want) {
		return nil
	}
	return fmt.Errorf("client %q screen still does not contain %q after widening the sidebar %d times (to its ceiling):\n%s", clientName, want, maxPresses, client.Frame(false))
}

// clientCreatesLongNamedShellSession is clientCreatesShellSession's sibling
// for a name deliberately long enough to push the row's status word past the
// sidebar's truncation budget (task 009 proves a non-default sidebar_width
// actually changes deck's own rendered frame, which needs a name long
// enough to be visibly cropped at the default width). It cannot reuse
// clientCreatesShellSession's own wait for "starting" on the row, since that
// word is exactly what a long name crops off screen; the row's second line
// ("created ...") never competes with the name for the same truncation
// budget, so it waits for that instead. It uses "/tmp" as the working
// directory rather than creating a scenario-scoped one, since this step
// only cares about the rendered row's shape, not the session's filesystem
// behaviour.
func clientCreatesLongNamedShellSession(ctx context.Context, clientName, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t/tmp\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "created ")
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
