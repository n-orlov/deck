package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateBlankNameSteps backs PRD requirement 6, SPEC.md:174-176:
// a blank name in the create modal is filled in, not rejected, defaulting
// to `<workspace>-<MMDD-HHMM>` from the wall clock (deterministic under a
// frozen DECK_CLOCK), with a collision appending the smallest free `-2`,
// `-3`, ... suffix rather than failing the create (task 014).
func registerCreateBlankNameSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started in a fresh directory labelled "([^"]+)" with the clock frozen at "([^"]+)"$`, clientStartedInFreshDirectoryLabelledWithFrozenClock)
	sc.Step(`^deck client "([^"]+)" submits the create modal with a blank name$`, clientSubmitsCreateModalWithBlankName)
	sc.Step(`^the state database has exactly one session in the directory labelled "([^"]+)", named "([^"]+)"$`, databaseHasOneSessionInDirectoryNamed)
	sc.Step(`^the state database has sessions named "([^"]+)" and "([^"]+)", both with cwd exactly the directory labelled "([^"]+)"$`, databaseHasTwoSessionsNamedInDirectory)
}

// clientStartedInFreshDirectoryLabelledWithFrozenClock is
// clientStartedInFreshDirectoryLabelled (features/create_session_test.go)
// plus a frozen DECK_CLOCK set on the scenario's clientEnv before the
// client starts, so every create in this scenario resolves
// m.settings.Clock.Now() to exactly iso -- the deterministic
// wall-clock-derived default SPEC.md:176 requires, rather than a value
// that would flake with real wall time.
func clientStartedInFreshDirectoryLabelledWithFrozenClock(ctx context.Context, name, label, iso string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_CLOCK="+iso)
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	client, err := h.StartNamedClientInDir(ctx, name, dir)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientSubmitsCreateModalWithBlankName presses enter on the create
// modal's default (name) field with nothing typed: no name field edit, no
// cwd field edit, so the field the modal opened with (blank name, cwd
// prefilled per SPEC §11.7) is exactly what submitCreate resolves the
// default from. Waits for the modal to close (its own title leaving the
// frame) rather than for the new row's "starting" status text: a fast
// tmux launch can carry a session straight past "starting" to "running"
// before this step's own poll ever samples the frame -- observed directly
// (task 014) on a scenario's *second* create, once tmux is already warm --
// so waiting for a status string that is allowed to have already been and
// gone is not reliable, while the modal's own title disappearing is: it is
// set and cleared exactly once, by submitCreate/the shellCreated success
// path, with no transient window to race.
func clientSubmitsCreateModalWithBlankName(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	if err := waitForFrameGone(ctx, client, "Create shell session"); err != nil {
		return err
	}
	// Give the newly-created session's own recent_cwds promotion (task 007)
	// a moment to land before a following step reopens the create modal --
	// otherwise a second blank-name create in the same scenario could race
	// the write and see stale (or no) history.
	time.Sleep(75 * time.Millisecond)
	return nil
}

// waitForFrameGone is WaitForFrame's negation: it blocks until the current
// frame no longer contains unwanted, using the same done/updated select
// loop WaitForFrame itself uses, so a caller can prove a transient title
// or message actually cleared rather than merely happening not to be
// present at one instant (features/agent_steps_test.go's
// clientScreenDoesNotContain, by contrast, is a single unpolled snapshot --
// fine for asserting an absence throughout a scenario, wrong for waiting
// on one to *become* absent).
func waitForFrameGone(ctx context.Context, d *ScreenDriver, unwanted string) error {
	for {
		if frame := d.Frame(false); !strings.Contains(frame, unwanted) {
			return nil
		}
		select {
		case <-d.done:
			return fmt.Errorf("deck exited before %q left the frame: %v\nframe:\n%s", unwanted, d.processError(), d.Frame(false))
		case <-d.updated:
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %q to leave the frame: %w\nframe:\n%s", unwanted, ctx.Err(), d.Frame(false))
		}
	}
}

// databaseHasOneSessionInDirectoryNamed asserts there is exactly one
// sessions row whose cwd is the real path registered under label, and
// that its name is exactly want -- the exact generated name, read from the
// store rather than only the screen (successCriteria for task 014).
func databaseHasOneSessionInDirectoryNamed(ctx context.Context, label, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT name FROM sessions WHERE cwd = ?", dir)
	if err != nil {
		return fmt.Errorf("query sessions in %q: %w", dir, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var got string
		if err := rows.Scan(&got); err != nil {
			return fmt.Errorf("scan session name: %w", err)
		}
		names = append(names, got)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(names) != 1 {
		return fmt.Errorf("sessions in directory %q = %v, want exactly one", dir, names)
	}
	if names[0] != want {
		return fmt.Errorf("session name = %q, want exactly %q", names[0], want)
	}
	return nil
}

// databaseHasTwoSessionsNamedInDirectory asserts both wantFirst and
// wantSecond exist as session names, and both rows' cwd is exactly the real
// path registered under label -- proving the collision suffix landed on a
// second create in the very same directory rather than merely existing
// somewhere in the store.
func databaseHasTwoSessionsNamedInDirectory(ctx context.Context, wantFirst, wantSecond, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, want := range []string{wantFirst, wantSecond} {
		var gotCWD string
		if err := db.QueryRowContext(ctx, "SELECT cwd FROM sessions WHERE name = ?", want).Scan(&gotCWD); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("no session named %q in the state database", want)
			}
			return fmt.Errorf("observe cwd for session %q: %w", want, err)
		}
		if gotCWD != dir {
			return fmt.Errorf("session %q cwd = %q, want exactly %q", want, gotCWD, dir)
		}
	}
	return nil
}
