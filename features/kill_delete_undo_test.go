package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/config"
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
	sc.Step(`^deck client "([^"]+)" is started with a short delete grace window$`, clientStartedWithShortDeleteGraceWindow)
	sc.Step(`^deck client "([^"]+)" is started with the clock frozen at "([^"]+)" and short undo and delete windows$`, clientStartedWithFrozenClockAndShortUndoAndDeleteWindows)
	sc.Step(`^deck client "([^"]+)" presses u$`, clientPressesUndo)
	sc.Step(`^(\d+) milliseconds pass$`, millisecondsPass)
	sc.Step(`^deck client "([^"]+)" presses d$`, clientPressesD)
	sc.Step(`^deck client "([^"]+)" presses dd$`, clientPressesDD)
	sc.Step(`^deck client "([^"]+)" clears the pending delete indicator with escape$`, clientClearsPendingDeleteWithEscape)
	sc.Step(`^deck client "([^"]+)" clears the pending delete indicator by pressing "([^"]+)"$`, clientClearsPendingDeleteWithKey)
	sc.Step(`^the state database session "([^"]+)" is tombstoned$`, stateDatabaseSessionIsTombstoned)
	sc.Step(`^the state database session "([^"]+)" is not tombstoned$`, stateDatabaseSessionIsNotTombstoned)
	sc.Step(`^the state database session "([^"]+)" is reaped$`, stateDatabaseSessionIsReaped)
	sc.Step(`^deck client "([^"]+)" seeds captures and a history file for session "([^"]+)"$`, clientSeedsCapturesAndHistoryFileForSession)
	sc.Step(`^the captures directory and history file for reaped session "([^"]+)" are gone$`, capturesDirAndHistoryFileForReapedSessionAreGone)
	sc.Step(`^the audit log still contains an earlier event for reaped session "([^"]+)"$`, auditLogStillContainsEarlierEventForReapedSession)
}

// clientSeedsCapturesAndHistoryFileForSession seeds the two per-session
// filesystem locations task 107's reap removes -- config.CapturesDir and
// config.HistoryFile, SPEC §9.4's captured scrollback and history file --
// since nothing in this tree writes either one yet (that is Phase 6's own
// deliverable). It also remembers the session's durable store id under its
// display name (h.capturedSessionIDs), because once dd's reap removes the
// sessions row entirely a later step can no longer look the id up by name.
func clientSeedsCapturesAndHistoryFileForSession(ctx context.Context, clientName, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var id string
	if err := db.QueryRowContext(ctx, `SELECT id FROM sessions WHERE name = ?`, sessionName).Scan(&id); err != nil {
		return fmt.Errorf("look up session %q id: %w", sessionName, err)
	}
	if h.capturedSessionIDs == nil {
		h.capturedSessionIDs = make(map[string]string)
	}
	h.capturedSessionIDs[sessionName] = id
	capturesDir := config.CapturesDir(h.Home, id)
	if err := os.MkdirAll(capturesDir, 0o700); err != nil {
		return fmt.Errorf("seed captures dir for session %q: %w", sessionName, err)
	}
	if err := os.WriteFile(filepath.Join(capturesDir, "scrollback"), []byte("replay me\n"), 0o600); err != nil {
		return fmt.Errorf("seed captures file for session %q: %w", sessionName, err)
	}
	historyFile := config.HistoryFile(h.Home, id)
	if err := os.MkdirAll(filepath.Dir(historyFile), 0o700); err != nil {
		return fmt.Errorf("seed history dir for session %q: %w", sessionName, err)
	}
	if err := os.WriteFile(historyFile, []byte("cd /work\n"), 0o600); err != nil {
		return fmt.Errorf("seed history file for session %q: %w", sessionName, err)
	}
	return nil
}

// capturesDirAndHistoryFileForReapedSessionAreGone polls -- the reap that
// removes these paths runs on the same asynchronous deleteGraceExpired tick
// stateDatabaseSessionIsReaped already polls for -- until neither the
// captures directory nor the history file exists, using the id
// clientSeedsCapturesAndHistoryFileForSession stored before the row itself
// became unqueryable by name.
func capturesDirAndHistoryFileForReapedSessionAreGone(ctx context.Context, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, ok := h.capturedSessionIDs[sessionName]
	if !ok {
		return fmt.Errorf("no captured session id for %q -- seed captures/history first", sessionName)
	}
	capturesDir := config.CapturesDir(h.Home, id)
	historyFile := config.HistoryFile(h.Home, id)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, capturesErr := os.Stat(capturesDir)
		_, historyErr := os.Stat(historyFile)
		if os.IsNotExist(capturesErr) && os.IsNotExist(historyErr) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("captures dir stat = %v, history file stat = %v, want both IsNotExist", capturesErr, historyErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// auditLogStillContainsEarlierEventForReapedSession proves the one
// deliberate exception to "reap leaves no trace": the JSONL audit log
// keeps this session's earlier records (its own "starting" transition,
// written well before dd/reap ran) even after the sessions row and its
// events rows are gone, per SPEC §9.2's "a log that rewrites itself when a
// row is deleted is not a log".
func auditLogStillContainsEarlierEventForReapedSession(ctx context.Context, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, ok := h.capturedSessionIDs[sessionName]
	if !ok {
		return fmt.Errorf("no captured session id for %q -- seed captures/history first", sessionName)
	}
	records, err := readAudit(h)
	if err != nil {
		return err
	}
	for _, record := range records {
		var event, recordSessionID string
		_ = json.Unmarshal(record["event"], &event)
		_ = json.Unmarshal(record["session_id"], &recordSessionID)
		if event == "starting" && recordSessionID == id {
			return nil
		}
	}
	return fmt.Errorf("audit log has no earlier \"starting\" record for reaped session %q (id %q)", sessionName, id)
}

// clientStartedWithShortDeleteGraceWindow mirrors
// clientStartedWithShortUndoWindow exactly (task 106): DECK_DELETE_GRACE_MS
// short enough that a scenario can wait out dd's own undo/reap window in
// real wall-clock time, since that window is scheduled via a real
// tea.Tick, not the frozen DECK_CLOCK (see internal/tui.Model's
// sessionDeleted/deleteGraceExpired handling).
func clientStartedWithShortDeleteGraceWindow(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_DELETE_GRACE_MS=200")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientStartedWithFrozenClockAndShortUndoAndDeleteWindows backs
// requirement 2 (Phase 0 R7): DECK_CLOCK is frozen for the whole scenario
// (m.settings.Clock.Now() never advances), while DECK_UNDO_MS and
// DECK_DELETE_GRACE_MS are both short in REAL wall-clock terms -- proving
// both windows tick from a real tea.Tick/monotonic source that keeps
// advancing regardless of the frozen wall clock the rest of the UI
// renders, rather than from m.settings.Clock, which would never expire
// either window at all.
func clientStartedWithFrozenClockAndShortUndoAndDeleteWindows(ctx context.Context, name, iso string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_CLOCK="+iso, "DECK_UNDO_MS=200", "DECK_DELETE_GRACE_MS=200")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
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

// stateDatabaseSessionIsReaped polls -- task 106's DECK_DELETE_GRACE_MS
// expiry dispatches store.ReapSession from a real tea.Tick's Cmd on its
// own goroutine, which races this assertion exactly the way an in-flight
// hook or probe does elsewhere, unlike a synchronous in-Update mutation --
// until the row is gone from the sessions table entirely (not merely
// tombstoned), mirroring databaseSessionStatus's own poll-rather-than-
// read-once shape (features/assertions_test.go).
func stateDatabaseSessionIsReaped(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(5 * time.Second)
	var count int
	for {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE name = ?`, name).Scan(&count); err != nil {
			return fmt.Errorf("observe session %q count: %w", name, err)
		}
		if count == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q still present after reap deadline, count=%d", name, count)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
