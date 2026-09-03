package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readNonEmptyLines reads path (tolerating "does not exist yet") and
// returns its non-empty lines -- used below to prove a hook ran, and ran
// exactly once, from the file it wrote rather than from a mock call count.
func readNonEmptyLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestArchiveRunsSessionThenGlobalPostDestroyExactlyOnce proves task 014's
// half of SPEC \u00a79.2 for `A`: both the session's own post_destroy and the
// service's GlobalPostDestroy run, each exactly once, in session-then-
// global order, and each sees DECK_TEARDOWN_KIND=archive and this
// session's own DECK_SESSION_ID -- all observed through files the two hook
// scripts write themselves, never a mock call count.
//
// Ordering is enforced structurally, not just observed: the global hook's
// shell line only writes its own file when the session hook's file
// already exists, so a session-and-global-swapped bug would leave the
// global file missing rather than merely reordered in a timestamp.
func TestArchiveRunsSessionThenGlobalPostDestroyExactlyOnce(t *testing.T) {
	svc := newArchiveTestService(t)
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session-hook.out")
	globalFile := filepath.Join(dir, "global-hook.out")
	svc.GlobalPostDestroy = "test -f " + sessionFile + " && echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + globalFile

	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "archive-hook-order", CWD: t.TempDir(),
		PostDestroy: "echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + sessionFile,
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}
	stopped, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	if msg, err := svc.Archive(context.Background(), stopped); err != nil || msg != "" {
		t.Fatalf("archive: msg=%q err=%v, want empty message and no error (both hooks succeed)", msg, err)
	}

	want := []string{"archive:" + session.ID}
	if got := readNonEmptyLines(t, sessionFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("session post_destroy file lines = %#v, want exactly %#v (ran once, correct env)", got, want)
	}
	if got := readNonEmptyLines(t, globalFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("global post_destroy file lines = %#v, want exactly %#v (ran once after the session hook, correct env)", got, want)
	}
}

// noteEventsForSession returns every "note"-kind event reason recorded for
// sessionID, newest first -- the durable record runOneTeardownHook writes
// via store.RecordSessionNote on a hook failure or timeout, read back from
// the real events table rather than asserted through a mock.
func noteEventsForSession(t *testing.T, svc Service, sessionID string) []string {
	t.Helper()
	rows, err := svc.Store.DB().QueryContext(context.Background(),
		`SELECT reason FROM events WHERE session_id = ? AND kind = 'note' ORDER BY seq DESC`, sessionID)
	if err != nil {
		t.Fatalf("query note events: %v", err)
	}
	defer rows.Close()
	var reasons []string
	for rows.Next() {
		var reason string
		if err := rows.Scan(&reason); err != nil {
			t.Fatalf("scan note event: %v", err)
		}
		reasons = append(reasons, reason)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate note events: %v", err)
	}
	return reasons
}

// TestDeleteFailingPostDestroyIsFailOpenAndTombstoneRecorded proves SPEC
// §9.2's fail-open contract for a non-zero-exit hook on the `dd` path: the
// tombstone Delete already wrote before running any hook survives the
// hook's own failure untouched (deleted_at stays set, the row stays absent
// from ListSessions -- never resurrected to undo a failed teardown), and
// the failure is recorded as a session-scoped "note" event naming which
// hook failed, all read back from the real store rather than a mock.
func TestDeleteFailingPostDestroyIsFailOpenAndTombstoneRecorded(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "delete-hook-fails", CWD: t.TempDir(),
		PostDestroy: "exit 7",
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}

	msg, err := svc.Delete(context.Background(), session)
	if err != nil {
		t.Fatalf("delete: %v, want no error (a teardown hook never blocks a teardown)", err)
	}
	if !strings.Contains(msg, "post_destroy failed") {
		t.Fatalf("delete message = %q, want it to name the failed hook", msg)
	}

	deleted, err := svc.Store.ListDeletedSessions(context.Background())
	if err != nil || len(deleted) != 1 || deleted[0].ID != session.ID || deleted[0].DeletedAt == 0 {
		t.Fatalf("ListDeletedSessions after failing hook = %#v, %v, want the tombstone durably written", deleted, err)
	}
	live, err := svc.Store.ListSessions(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("ListSessions after failing hook = %#v, %v, want empty (row never resurrected by a failed hook)", live, err)
	}

	notes := noteEventsForSession(t, svc, session.ID)
	if len(notes) != 1 || !strings.Contains(notes[0], "session post_destroy failed") {
		t.Fatalf("note events for session = %#v, want exactly one naming the session hook's failure", notes)
	}
}

// TestArchiveTimedOutPostDestroyIsKilledAndRecorded proves SPEC §9.2's
// timeout half of the same contract: a hook that outlives postDestroyTimeout
// is killed rather than left to run to completion, Archive still returns
// without error (fail-open), archived_at is durably set (the row is not
// resurrected), and the timeout is recorded as a "note" event naming the
// hook. The bound is shortened only by writing to postDestroyTimeout itself
// -- the same named value production uses -- restored via defer, never a
// second test-only knob or branch in product code; the hook line asks for
// far longer than the shortened bound so a test that let it run to
// completion (i.e. failed to actually kill it) would time out the whole
// `go test` run rather than pass by accident.
func TestArchiveTimedOutPostDestroyIsKilledAndRecorded(t *testing.T) {
	original := postDestroyTimeout
	postDestroyTimeout = 200 * time.Millisecond
	defer func() { postDestroyTimeout = original }()

	svc := newArchiveTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "archive-hook-times-out", CWD: t.TempDir(),
		PostDestroy: "sleep 30",
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}
	stopped, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	msg, err := svc.Archive(context.Background(), stopped)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("archive: %v, want no error (a teardown hook never blocks a teardown)", err)
	}
	if !strings.Contains(msg, "post_destroy timed out") {
		t.Fatalf("archive message = %q, want it to name the timed-out hook", msg)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("archive took %s, want it bounded by the shortened postDestroyTimeout (the sleep 30 hook must be killed, not awaited)", elapsed)
	}

	got, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArchivedAt == 0 {
		t.Fatalf("session archived_at = 0 after a timed-out hook, want it durably set (never resurrected)")
	}

	notes := noteEventsForSession(t, svc, session.ID)
	if len(notes) != 1 || !strings.Contains(notes[0], "session post_destroy timed out") {
		t.Fatalf("note events for session = %#v, want exactly one naming the session hook's timeout", notes)
	}
}

// TestDeleteRunsSessionThenGlobalPostDestroyExactlyOnce is the `dd`
// counterpart of the Archive test above: same session-then-global,
// exactly-once, DECK_TEARDOWN_KIND/DECK_SESSION_ID contract, this time
// with DECK_TEARDOWN_KIND=delete and Delete's own live-pane-kill-then-
// tombstone path ahead of the hooks.
func TestDeleteRunsSessionThenGlobalPostDestroyExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session-hook.out")
	globalFile := filepath.Join(dir, "global-hook.out")

	svc := newTombstoneTestService(t)
	svc.GlobalPostDestroy = "test -f " + sessionFile + " && echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + globalFile

	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "delete-hook-order", CWD: t.TempDir(),
		PostDestroy: "echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + sessionFile,
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}

	if msg, err := svc.Delete(context.Background(), session); err != nil || msg != "" {
		t.Fatalf("delete: msg=%q err=%v, want empty message and no error (both hooks succeed)", msg, err)
	}

	want := []string{"delete:" + session.ID}
	if got := readNonEmptyLines(t, sessionFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("session post_destroy file lines = %#v, want exactly %#v (ran once, correct env)", got, want)
	}
	if got := readNonEmptyLines(t, globalFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("global post_destroy file lines = %#v, want exactly %#v (ran once after the session hook, correct env)", got, want)
	}
}
