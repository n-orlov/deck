package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestDeleteKillsLivePaneAndTombstonesPreservingCWDAndConversation proves
// task 105's dd submit path: the pane is killed (real tmux has-session
// false afterwards), the row is tombstoned (deleted_at set, absent from
// ListSessions) rather than removed, and the cwd/conversation id it names
// as surviving are untouched.
func TestDeleteKillsLivePaneAndTombstonesPreservingCWDAndConversation(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	sentinel := filepath.Join(cwd, "deck-delete-sentinel")
	contents := []byte("user files are never owned by deck\n")
	if err := os.WriteFile(sentinel, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	clock, _ := config.NewClock("2025-01-02T03:04:05Z", "")
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-delete-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock, IDs: config.NewIDGenerator("delete-test"), Shell: "/bin/sh"}
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "keep cwd", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	live, err := svc.TMux.List(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("live sessions after delete = %#v, %v", live, err)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("ListSessions after delete = %#v, %v, want empty (tombstoned)", rows, err)
	}
	deleted, err := db.ListDeletedSessions(context.Background())
	if err != nil || len(deleted) != 1 || deleted[0].ID != session.ID || deleted[0].DeletedAt == 0 {
		t.Fatalf("ListDeletedSessions after delete = %#v, %v", deleted, err)
	}
	if deleted[0].ConversationID != session.ConversationID {
		t.Fatalf("conversation id changed by delete: before %q after %q", session.ConversationID, deleted[0].ConversationID)
	}
	if deleted[0].CWD != cwd {
		t.Fatalf("cwd changed by delete: before %q after %q", cwd, deleted[0].CWD)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("sentinel after delete = %q, %v", got, err)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'deleted'`, session.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("deleted event count = %d, %v", events, err)
	}
	log, err := os.ReadFile(logger.Path())
	if err != nil || !strings.Contains(string(log), `"event":"deleted"`) || !strings.Contains(string(log), session.ID) {
		t.Fatalf("delete audit = %q, %v", log, err)
	}
}

// TestDeleteRefusesEmptySessionID mirrors Kill's own guard: a caller
// cannot tombstone a session it never durably identified.
func TestDeleteRefusesEmptySessionID(t *testing.T) {
	svc := Service{}
	if _, err := svc.Delete(context.Background(), store.Session{}); err == nil {
		t.Fatal("Delete with empty session, want error")
	}
}

func newTombstoneTestService(t *testing.T) Service {
	home := t.TempDir()
	clock, _ := config.NewClock("2025-01-02T03:04:05Z", "")
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-restore-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	return Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock, IDs: config.NewIDGenerator("restore-test"), Shell: "/bin/sh", DeckHome: home}
}

// TestRestoreClearsTombstoneAndReturnsToListSessions proves task 106's `u`
// undo-of-a-delete service path: Restore clears deleted_at (the row is
// visible in ListSessions again) and records a "restored" audit
// transition, without ever touching the live pane (it was already killed
// or never existed; Restore never launches anything).
func TestRestoreClearsTombstoneAndReturnsToListSessions(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "restorable", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Restore(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.DeletedAt != 0 {
		t.Fatalf("restored session DeletedAt = %d, want 0", restored.DeletedAt)
	}
	rows, err := svc.Store.ListSessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].ID != session.ID {
		t.Fatalf("ListSessions after restore = %#v, %v, want the restored row", rows, err)
	}
	log, err := os.ReadFile(svc.Audit.Path())
	if err != nil || !strings.Contains(string(log), `"event":"restored"`) {
		t.Fatalf("restore audit = %q, %v", log, err)
	}
}

// TestRestoreRefusesEmptySessionID mirrors Delete's own guard.
func TestRestoreRefusesEmptySessionID(t *testing.T) {
	svc := Service{}
	if _, err := svc.Restore(context.Background(), ""); err == nil {
		t.Fatal("Restore with empty session id, want error")
	}
}

// TestReapRemovesTombstonedRowPermanently proves task 106's grace-window
// expiry service path: Reap only ever acts on a row Delete has already
// tombstoned (store.ReapSession's own contract), and afterward the row is
// gone from the store entirely, not merely hidden.
func TestReapRemovesTombstonedRowPermanently(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "reapable", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reap(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := svc.Store.DB().QueryRow(`SELECT count(*) FROM sessions WHERE id = ?`, session.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("session row count after reap = %d, %v, want 0", count, err)
	}
}

// TestReapRefusesEmptySessionID mirrors Delete's own guard.
func TestReapRefusesEmptySessionID(t *testing.T) {
	svc := Service{}
	if err := svc.Reap(context.Background(), ""); err == nil {
		t.Fatal("Reap with empty session id, want error")
	}
}

// TestReapRemovesCapturesDirAndHistoryFile proves task 107's own addition
// to Reap: SPEC §9.2's "deck's own per-session files, meaning §9.4's
// history file and captured scrollback" are removed by the reap, using
// exactly the two paths config.CapturesDir/config.HistoryFile define --
// nothing in this tree writes either path yet (Phase 6), so this test
// seeds them itself to prove the removal side independent of the writer.
func TestReapRemovesCapturesDirAndHistoryFile(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "reap-files", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	capturesDir := config.CapturesDir(svc.DeckHome, session.ID)
	if err := os.MkdirAll(capturesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(capturesDir, "scrollback"), []byte("replay me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyFile := config.HistoryFile(svc.DeckHome, session.ID)
	if err := os.MkdirAll(filepath.Dir(historyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyFile, []byte("cd /work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reap(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(capturesDir); !os.IsNotExist(err) {
		t.Fatalf("captures dir after reap: stat = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(historyFile); !os.IsNotExist(err) {
		t.Fatalf("history file after reap: stat = %v, want IsNotExist", err)
	}
}

// TestReapToleratesMissingCapturesDirAndHistoryFile proves the degrade-to-
// no-replay-never-to-an-error half of the same contract: for the common
// case today -- nothing has ever written either path, since Phase 6 is
// the only future writer -- Reap still succeeds.
func TestReapToleratesMissingCapturesDirAndHistoryFile(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "reap-no-files", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reap(context.Background(), session.ID); err != nil {
		t.Fatalf("Reap with no captures dir or history file present: %v, want nil", err)
	}
}

// TestReapLeavesEventsOutboxAndNotifyStateBehind proves requirement 24's
// row-level half of "leave no trace": events rows cascade away with the
// sessions row (ON DELETE CASCADE, schemaV1), and the notify_epoch/waiting
// state that lived on the sessions row itself goes with it -- there is no
// separate outbox table in this schema for anything to leave behind
// (docs/reports/phase3-findings.md records this so a later phase that
// adds one knows to extend this same reap path).
func TestReapLeavesEventsOutboxAndNotifyStateBehind(t *testing.T) {
	svc := newTombstoneTestService(t)
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "reap-cascade", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := svc.Store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, session.ID).Scan(&before); err != nil || before == 0 {
		t.Fatalf("events for session before reap = %d, %v, want > 0", before, err)
	}
	if err := svc.Reap(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := svc.Store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, session.ID).Scan(&after); err != nil || after != 0 {
		t.Fatalf("events for session after reap = %d, %v, want 0 (cascaded)", after, err)
	}
}

// TestPurgeRemovesExactlyTheDeclaredPathAndIsANoOpOnEmpty proves task
// 110's Purge contract: it removes exactly the path it is given -- never
// resolving, inferring or globbing one of its own -- and treats an empty
// path (the "cannot locate a transcript" case, task 109) as a no-op
// rather than an error, since the TUI never calls Purge with a non-empty
// path unless TranscriptPaths itself already located one.
func TestPurgeRemovesExactlyTheDeclaredPathAndIsANoOpOnEmpty(t *testing.T) {
	svc := newTombstoneTestService(t)

	if err := svc.Purge(context.Background(), ""); err != nil {
		t.Fatalf("Purge(\"\") = %v, want nil (no-op)", err)
	}

	dir := t.TempDir()
	transcript := filepath.Join(dir, "conversation.jsonl")
	sibling := filepath.Join(dir, "sibling.jsonl")
	if err := os.WriteFile(transcript, []byte("transcript\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("sibling\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := svc.Purge(context.Background(), transcript); err != nil {
		t.Fatalf("Purge(%q) = %v, want nil", transcript, err)
	}
	if _, err := os.Stat(transcript); !os.IsNotExist(err) {
		t.Fatalf("stat purged %q = %v, want IsNotExist", transcript, err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("unrelated file %q was touched by Purge: %v", sibling, err)
	}

	if err := svc.Purge(context.Background(), transcript); err == nil {
		t.Fatal("Purge of an already-purged path returned nil, want an error")
	}
}
