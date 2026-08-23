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
	if err := svc.Delete(context.Background(), session); err != nil {
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
	if err := svc.Delete(context.Background(), store.Session{}); err == nil {
		t.Fatal("Delete with empty session, want error")
	}
}
