package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	sc.Step(`^the fake claude transcript for session "([^"]+)" is captured as "([^"]+)"$`, fakeClaudeTranscriptForSessionIsCapturedAs)
	sc.Step(`^the transcript captured as "([^"]+)" still exists byte-identical$`, transcriptCapturedStillExistsByteIdentical)
	sc.Step(`^the transcript captured as "([^"]+)" no longer exists$`, transcriptCapturedNoLongerExists)
	sc.Step(`^deck client "([^"]+)" archives its selected session "([^"]+)"$`, clientArchivesSelectedSession)
	sc.Step(`^deck client "([^"]+)" presses A on its selected session "([^"]+)"$`, clientPressesArchiveOnSelectedSession)
	sc.Step(`^deck client "([^"]+)" presses "([^"]+)" again inside the archive confirm for "([^"]+)"$`, clientPressesKeyAgainInsideArchiveConfirm)
	sc.Step(`^deck client "([^"]+)" undoes the archive with u for "([^"]+)"$`, clientUndoesArchiveWithU)
	sc.Step(`^the state database session "([^"]+)" is archived$`, stateDatabaseSessionIsArchived)
	sc.Step(`^the state database session "([^"]+)" is not archived$`, stateDatabaseSessionIsNotArchived)
}

// transcriptSnapshot is task 110's own fixture record, keyed by an
// operator-chosen label (registerKillDeleteUndoSteps' transcriptSnapshots):
// the exact absolute path an agent's own declared TranscriptPaths
// convention (internal/agent, task 109) resolved to, and the bytes it held
// at capture time, so a later step can assert either "still there,
// byte-identical" (requirement 25, no purge) or "gone" (requirement 26,
// purge chosen) without re-deriving the path from a session row that dd's
// own reap may since have removed entirely.
type transcriptSnapshot struct {
	path    string
	content []byte
}

// claudeTranscriptPathForSession mirrors sessionLastMessage's own path
// convention exactly (cmd/fake-claude's transcriptPath / internal/agent's
// Claude.TranscriptPaths, task 109's provenance): $HOME/.claude/projects/
// <cwd, every path separator replaced with "-">/<conversation id>.jsonl,
// against the scenario's own fixture HOME (h.agentHOMEDir) and working
// directory (h.workingDir) rather than the real developer's.
func claudeTranscriptPathForSession(h *ScenarioHarness, name string) (string, error) {
	if h.agentHOMEDir == "" {
		return "", fmt.Errorf("fixture HOME directory was not configured (call the fake claude PATH step first)")
	}
	conversationID, err := sessionConversationID(h, name)
	if err != nil {
		return "", err
	}
	if conversationID == "" {
		return "", fmt.Errorf("session %q has no conversation id", name)
	}
	project := strings.ReplaceAll(h.workingDir, string(os.PathSeparator), "-")
	return filepath.Join(h.agentHOMEDir, ".claude", "projects", project, conversationID+".jsonl"), nil
}

// fakeClaudeTranscriptForSessionIsCapturedAs resolves session's declared
// transcript path and snapshots its current bytes under label, before dd's
// own kill/reap can touch anything -- once the row is tombstoned/reaped
// the conversation id can no longer be looked up by name, exactly like
// clientSeedsCapturesAndHistoryFileForSession's own capturedSessionIDs
// capture above.
func fakeClaudeTranscriptForSessionIsCapturedAs(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path, err := claudeTranscriptPathForSession(h, name)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read transcript %q for session %q: %w", path, name, err)
	}
	if h.transcriptSnapshots == nil {
		h.transcriptSnapshots = make(map[string]transcriptSnapshot)
	}
	h.transcriptSnapshots[label] = transcriptSnapshot{path: path, content: content}
	return nil
}

// transcriptCapturedStillExistsByteIdentical proves requirement 25: a dd
// delete (and its eventual reap) without purge never touches the agent's
// own transcript file -- the exact bytes captured before dd ran are still
// there afterwards, at the exact same path.
func transcriptCapturedStillExistsByteIdentical(ctx context.Context, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	snapshot, ok := h.transcriptSnapshots[label]
	if !ok {
		return fmt.Errorf("no transcript was captured as %q", label)
	}
	got, err := os.ReadFile(snapshot.path)
	if err != nil {
		return fmt.Errorf("read transcript %q captured as %q: %w", snapshot.path, label, err)
	}
	if string(got) != string(snapshot.content) {
		return fmt.Errorf("transcript %q captured as %q changed:\nbefore: %q\nafter:  %q", snapshot.path, label, snapshot.content, got)
	}
	return nil
}

// transcriptCapturedNoLongerExists proves requirement 26's other half:
// choosing purge in the delete confirm removed exactly the declared
// transcript file captured under label.
func transcriptCapturedNoLongerExists(ctx context.Context, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	snapshot, ok := h.transcriptSnapshots[label]
	if !ok {
		return fmt.Errorf("no transcript was captured as %q", label)
	}
	if _, err := os.Stat(snapshot.path); err == nil {
		return fmt.Errorf("transcript %q captured as %q still exists, want purged", snapshot.path, label)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat purged transcript %q captured as %q: %w", snapshot.path, label, err)
	}
	return nil
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

// waitForSessionColumnState is the one shared bounded-wait for every
// state-database assertion reachable immediately after an async keypress
// (task 204, review finding 1): u's undo, dd's tombstone, and A's archive
// all dispatch their store mutation from a goroutine (an internal/service
// Cmd, a tea.Tick callback, ...) that races whatever step comes next in the
// .feature file, exactly the way stateDatabaseSessionIsReaped's own poll
// already accounted for -- a single immediate read merely gets lucky most
// of the time. Stability run 7 caught exactly this for
// stateDatabaseSessionIsNotTombstoned, reading the DB once right after the
// `u` keypress and losing the race
// (docs/reports/phase3g-112-stability10/run-7.log:5019):
//
//	Error: after scenario hook failed: session "filter-dd-archived" has deleted_at=1787938118179, want not tombstoned
//
// column identifies the table field purely for the timeout message; get is
// the column's existing single-read accessor (sessionDeletedAt,
// sessionArchivedAt, ...); satisfied reports whether the observed value is
// the one the step wants. No assertion is weakened: once the deadline
// passes the last observed value is still checked and still fails, with a
// clear message naming both the column and the value actually observed.
func waitForSessionColumnState(ctx context.Context, name, column string, timeout time.Duration, get func(context.Context, string) (int64, error), satisfied func(int64) bool, want string) error {
	deadline := time.Now().Add(timeout)
	for {
		value, err := get(ctx, name)
		if err != nil {
			return err
		}
		if satisfied(value) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q still has %s=%d after %s, want %s", name, column, value, timeout, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func stateDatabaseSessionIsTombstoned(ctx context.Context, name string) error {
	return waitForSessionColumnState(ctx, name, "deleted_at", 3*time.Second, sessionDeletedAt, func(v int64) bool { return v != 0 }, "tombstoned")
}

func stateDatabaseSessionIsNotTombstoned(ctx context.Context, name string) error {
	return waitForSessionColumnState(ctx, name, "deleted_at", 3*time.Second, sessionDeletedAt, func(v int64) bool { return v == 0 }, "not tombstoned")
}

// clientArchivesSelectedSession drives R72's REAL confirm dialog (issue #10,
// SPEC.md:752) end to end through the keymap: a real `A` keypress, an
// assertion that the confirm naming this session is actually up (so the
// keypress genuinely went through the keymap and opened the dialog rather
// than being swallowed), the real confirm key, and only then the wait for the
// row to leave the frame. It deliberately does NOT call the archive service
// directly and does NOT auto-confirm behind the scenes: either shortcut would
// leave the confirm untested by every scenario except its own, which is the
// repair the PRD calls out as failing review.
//
// Whether the row was already stopped (archived_at set alone) or not
// (kill-and-archive as one action), an archived row is hidden from
// ListSessions' default view, so the render this ends on is exactly the one
// requirement 27 requires -- and it also proves the dialog itself closed,
// since the dialog names the session too.
func clientArchivesSelectedSession(ctx context.Context, name, sessionName string) error {
	if err := clientPressesArchiveOnSelectedSession(ctx, name, sessionName); err != nil {
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
	if err := client.Send("\r"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.WaitForFrameGone(wait, false, sessionName)
}

// clientPressesArchiveOnSelectedSession is the first half alone: press `A`
// and wait for the confirm that names the session, writing nothing. It is its
// own step so a scenario can assert what the unconfirmed keypress did NOT do
// -- the whole point of R72.
func clientPressesArchiveOnSelectedSession(ctx context.Context, name, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("A"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.WaitForFrame(wait, false, "Archive "+sessionName)
}

// clientPressesKeyAgainInsideArchiveConfirm proves the dialog suppresses the
// bare-letter keymap while open: the key is sent for real, then the harness
// waits for the pty to go quiet (the same quiescence wait
// clientCapturesSettledFrameAs uses -- a key the TUI deliberately ignores
// produces no render to wait ON, so "nothing happened" can only be asserted
// once any in-flight render has landed) and the confirm must still be up,
// naming the same session. The scenario pairs this with a database assertion
// that nothing was written.
func clientPressesKeyAgainInsideArchiveConfirm(ctx context.Context, name, key, sessionName string) error {
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
	frame, err := client.WaitForQuiescence(ctx, false, captureSettledQuietWindow)
	if err != nil {
		return err
	}
	if !strings.Contains(frame, "Archive "+sessionName) {
		return fmt.Errorf("%q inside the archive confirm left the dialog for %q:\n%s", key, sessionName, frame)
	}
	if !strings.Contains(frame, "Enter archives") {
		return fmt.Errorf("%q inside the archive confirm dropped the confirm's own keys:\n%s", key, frame)
	}
	return nil
}

// clientUndoesArchiveWithU drives R72's success toast's own offer (issue #10,
// SPEC.md:752): a real `u` keypress on the real keymap -- no service call, no
// `U` on a row the operator cannot even see any more -- bounded on the
// observable consequence, archived_at going back to 0, exactly as
// clientUnarchivesSelectedSession bounds `U`. The row is absent from the
// default list at the moment `u` is pressed, which is the point: the undo
// window remembers what to unarchive, so the operator does not have to find it
// inside the `/` filter first.
func clientUndoesArchiveWithU(ctx context.Context, name, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("u"); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		archivedAt, err := sessionArchivedAt(ctx, sessionName)
		if err != nil {
			return err
		}
		if archivedAt == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q still has archived_at=%d after the archive toast's own u\nframe:\n%s", sessionName, archivedAt, client.Frame(false))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// sessionArchivedAt/stateDatabaseSessionIsArchived/
// stateDatabaseSessionIsNotArchived mirror sessionDeletedAt's tombstone
// shape exactly, but read archived_at (task 111) -- a flag, never a
// status, so these never touch or infer anything about the status column.
func sessionArchivedAt(ctx context.Context, name string) (int64, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return 0, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var archivedAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT archived_at FROM sessions WHERE name = ?`, name).Scan(&archivedAt); err != nil {
		return 0, fmt.Errorf("observe session %q archived_at: %w", name, err)
	}
	return archivedAt.Int64, nil
}

func stateDatabaseSessionIsArchived(ctx context.Context, name string) error {
	return waitForSessionColumnState(ctx, name, "archived_at", 3*time.Second, sessionArchivedAt, func(v int64) bool { return v != 0 }, "archived")
}

func stateDatabaseSessionIsNotArchived(ctx context.Context, name string) error {
	return waitForSessionColumnState(ctx, name, "archived_at", 3*time.Second, sessionArchivedAt, func(v int64) bool { return v == 0 }, "not archived")
}

// sessionRowCount backs stateDatabaseSessionIsReaped below -- task 106's
// DECK_DELETE_GRACE_MS expiry dispatches store.ReapSession from a real
// tea.Tick's Cmd on its own goroutine, which races this assertion exactly
// the way an in-flight hook or probe does elsewhere, unlike a synchronous
// in-Update mutation -- until the row is gone from the sessions table
// entirely (not merely tombstoned), mirroring databaseSessionStatus's own
// poll-rather-than-read-once shape (features/assertions_test.go).
func sessionRowCount(ctx context.Context, name string) (int64, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return 0, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var count int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE name = ?`, name).Scan(&count); err != nil {
		return 0, fmt.Errorf("observe session %q count: %w", name, err)
	}
	return count, nil
}

func stateDatabaseSessionIsReaped(ctx context.Context, name string) error {
	return waitForSessionColumnState(ctx, name, "count(*)", 5*time.Second, sessionRowCount, func(v int64) bool { return v == 0 }, "reaped (count=0)")
}
