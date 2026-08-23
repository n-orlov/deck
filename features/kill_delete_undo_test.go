package features

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerKillDeleteUndoSteps backs PRD requirement 22: x still kills with
// no confirmation and still refuses an already-stopped row, but a
// successful kill leaves a toast naming undo, actionable for exactly
// DECK_UNDO_MS, that u resumes; once the window is gone (expired, or
// already spent by an earlier u), u does nothing (task 102). Task 105
// adds the dd tombstone scenarios to this same file; task 106 adds the
// grace-window/reap scenarios.
func registerKillDeleteUndoSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with a short undo window$`, clientStartedWithShortUndoWindow)
	sc.Step(`^deck client "([^"]+)" presses u$`, clientPressesUndo)
	sc.Step(`^(\d+) milliseconds pass$`, millisecondsPass)
	sc.Step(`^deck client "([^"]+)" presses d$`, clientPressesD)
	sc.Step(`^deck client "([^"]+)" presses dd$`, clientPressesDD)
	sc.Step(`^deck client "([^"]+)" clears the pending delete indicator with escape$`, clientClearsPendingDeleteWithEscape)
	sc.Step(`^deck client "([^"]+)" clears the pending delete indicator by pressing "([^"]+)"$`, clientClearsPendingDeleteWithKey)
	sc.Step(`^the state database session "([^"]+)" is tombstoned$`, stateDatabaseSessionIsTombstoned)
	sc.Step(`^the state database session "([^"]+)" is not tombstoned$`, stateDatabaseSessionIsNotTombstoned)
}

// clientStartedWithShortUndoWindow sets DECK_UNDO_MS short enough that a
// scenario can wait it out in real wall-clock time (the undo toast's expiry
// is scheduled via a real tea.Tick, not the frozen DECK_CLOCK -- see
// internal/tui.Model's sessionKilled/undoExpired handling) without the
// scenario itself taking anywhere near the real 10s default.
func clientStartedWithShortUndoWindow(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_UNDO_MS=200")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientPressesUndo sends u -- requirement 22's undo key, which always
// targets the most recently killed session (internal/tui.Model's
// undoSessionID), never the current selection.
func clientPressesUndo(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return client.Send("u")
}

// millisecondsPass is a real-wall-clock wait, not a frozen-clock advance:
// the undo toast's expiry is scheduled via a real tea.Tick against
// DECK_UNDO_MS (internal/tui.Model's sessionKilled/undoExpired handling),
// never against m.settings.Clock, so a scenario proving the window has
// expired must wait it out for real.
func millisecondsPass(ctx context.Context, ms int) error {
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// clientClearsPendingDeleteWithEscape sends Esc while the first-`d`
// pending indicator is showing and waits until the render has actually
// dropped it -- WaitForFrameGone, not "deck - sessions" (already on
// screen underneath the note, so waiting for it proves nothing about
// whether Esc's own re-render has happened yet).
func clientClearsPendingDeleteWithEscape(ctx context.Context, name string) error {
	return clientClearsPendingDeleteWithKey(ctx, name, "\x1b")
}

func clientClearsPendingDeleteWithKey(ctx context.Context, name, key string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send(key); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.WaitForFrameGone(wait, false, "again")
}

// clientPressesD sends a single `d` -- task 105's first half of the dd
// chord: a visible pending-delete indicator, changing nothing in the
// store.
func clientPressesD(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("d"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "again")
}

// clientPressesDD sends `d` twice in a row -- task 105's confirm dialog,
// which names what survives (the conversation and the working directory)
// before Enter tombstones the row. It waits for the first `d`'s own
// pending-indicator render before sending the second: two literal `d`
// bytes written back-to-back with no gap can land in the same PTY read as
// a synthetic burst no real keyboard ever produces, which this driver's
// underlying ANSI decoder does not reliably split into two key events --
// so, unlike a human pressing the chord, this needs the intermediate
// render as the synchronization point instead.
func clientPressesDD(ctx context.Context, name string) error {
	if err := clientPressesD(ctx, name); err != nil {
		return err
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("d"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return client.WaitForFrame(wait, false, "Enter deletes")
}

// stateDatabaseSessionIsTombstoned/stateDatabaseSessionIsNotTombstoned read
// the raw deleted_at column directly (never internal/store's Go types,
// never ListSessions, which is the very thing under test here -- it is
// what excludes a tombstoned row from the default view).
func sessionDeletedAt(ctx context.Context, name string) (int64, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return 0, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var deletedAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT deleted_at FROM sessions WHERE name = ?`, name).Scan(&deletedAt); err != nil {
		return 0, fmt.Errorf("observe session %q deleted_at: %w", name, err)
	}
	return deletedAt.Int64, nil
}

func stateDatabaseSessionIsTombstoned(ctx context.Context, name string) error {
	deletedAt, err := sessionDeletedAt(ctx, name)
	if err != nil {
		return err
	}
	if deletedAt == 0 {
		return fmt.Errorf("session %q has deleted_at=0, want tombstoned", name)
	}
	return nil
}

func stateDatabaseSessionIsNotTombstoned(ctx context.Context, name string) error {
	deletedAt, err := sessionDeletedAt(ctx, name)
	if err != nil {
		return err
	}
	if deletedAt != 0 {
		return fmt.Errorf("session %q has deleted_at=%d, want not tombstoned", name, deletedAt)
	}
	return nil
}
