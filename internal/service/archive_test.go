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

func newArchiveTestService(t *testing.T) Service {
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
	socket := "deck-archive-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	return Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock, IDs: config.NewIDGenerator("archive-test"), Shell: "/bin/sh", DeckHome: home}
}

// TestArchiveOnStoppedSessionOnlySetsArchivedAtAndLeavesStatus proves
// requirement 27's plain case: a stopped session is archived without ever
// touching tmux (there is no live pane to kill) and without its Status
// changing away from "stopped" -- archived_at is a flag, never a status.
func TestArchiveOnStoppedSessionOnlySetsArchivedAtAndLeavesStatus(t *testing.T) {
	svc := newArchiveTestService(t)
	cwd := t.TempDir()
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "already-stopped", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}
	stopped, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Archive(context.Background(), stopped); err != nil {
		t.Fatalf("archive: %v", err)
	}

	got, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("Status after archive = %q, want unchanged %q", got.Status, "stopped")
	}
	if got.ArchivedAt == 0 {
		t.Fatal("ArchivedAt after archive = 0, want set")
	}
	rows, err := svc.Store.ListSessions(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("ListSessions after archive = %#v, %v, want empty (archived hides from the default list)", rows, err)
	}
}

// TestArchiveOnNonStoppedSessionKillsThenArchivesAsOneAction proves
// requirement 27's "kill and archive" case: a live session is offered a
// single action rather than a refusal -- the pane is killed (real
// has-session false afterward), Status becomes "stopped" through the same
// path a plain kill takes, and archived_at is set in the same call.
func TestArchiveOnNonStoppedSessionKillsThenArchivesAsOneAction(t *testing.T) {
	svc := newArchiveTestService(t)
	cwd := t.TempDir()
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "still-running", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if session.Status == "stopped" {
		t.Fatalf("fixture session unexpectedly already stopped: %+v", session)
	}

	if err := svc.Archive(context.Background(), session); err != nil {
		t.Fatalf("archive (kill and archive): %v", err)
	}

	live, err := svc.TMux.List(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("live sessions after kill-and-archive = %#v, %v, want none", live, err)
	}
	got, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("Status after kill-and-archive = %q, want %q", got.Status, "stopped")
	}
	if got.ArchivedAt == 0 {
		t.Fatal("ArchivedAt after kill-and-archive = 0, want set")
	}
	if !got.KilledByUser {
		t.Fatal("KilledByUser after kill-and-archive = false, want true (same path a plain kill takes)")
	}

	log, err := os.ReadFile(svc.Audit.Path())
	if err != nil || !strings.Contains(string(log), `"event":"killed"`) || !strings.Contains(string(log), `"event":"archived"`) {
		t.Fatalf("kill-and-archive audit = %q, %v, want both killed and archived transitions", log, err)
	}
}

// TestArchiveRefusesEmptySessionID mirrors Kill/Delete's own guard: a
// caller cannot archive a session it never durably identified.
func TestArchiveRefusesEmptySessionID(t *testing.T) {
	svc := Service{}
	if err := svc.Archive(context.Background(), store.Session{}); err == nil {
		t.Fatal("Archive with empty session, want error")
	}
}
