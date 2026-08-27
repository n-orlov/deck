package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// seedSessionFiles writes the two deck-owned per-session paths SPEC §9.2
// names -- the captures directory and §9.4's history file -- for a session
// id. Nothing in this tree writes either one yet (Phase 6 is the future
// writer), exactly as TestReapRemovesCapturesDirAndHistoryFile already
// does, so a test that wants to prove the REMOVAL side seeds them itself.
func seedSessionFiles(t *testing.T, svc Service, sessionID string) (string, string) {
	t.Helper()
	capturesDir := config.CapturesDir(svc.DeckHome, sessionID)
	if err := os.MkdirAll(capturesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(capturesDir, "scrollback"), []byte("replay me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyFile := config.HistoryFile(svc.DeckHome, sessionID)
	if err := os.MkdirAll(filepath.Dir(historyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyFile, []byte("cd /work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return capturesDir, historyFile
}

// TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles proves
// the service half of R77 (SPEC §9.2): taking a deleted session's name
// reaps that session, and a reap is not finished when only its row is
// gone -- deck's own per-session files (captures dir, §9.4 history file)
// go with it. The store does the row/event removal inside CreateSession's
// own transaction; the files are removed here, AFTER that commit.
func TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles(t *testing.T) {
	svc := newTombstoneTestService(t)
	ctx := context.Background()
	first, err := svc.CreateShell(ctx, ShellCreateInput{Name: "reused name", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	capturesDir, historyFile := seedSessionFiles(t, svc, first.ID)
	if err := svc.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateShell(ctx, ShellCreateInput{Name: "reused name", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateShell reusing a tombstoned name: %v, want success", err)
	}
	if second.ID == first.ID {
		t.Fatalf("reuse returned the old row: id %q", second.ID)
	}
	exists, err := svc.Store.SessionRowExists(ctx, first.ID)
	if err != nil || exists {
		t.Fatalf("tombstoned holder row after reuse: exists = %v, %v, want gone", exists, err)
	}
	if _, err := os.Stat(capturesDir); !os.IsNotExist(err) {
		t.Fatalf("captures dir of the reaped holder after reuse: stat = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(historyFile); !os.IsNotExist(err) {
		t.Fatalf("history file of the reaped holder after reuse: stat = %v, want IsNotExist", err)
	}
	// The new session's own files were never touched by the cleanup.
	if _, err := os.Stat(config.CapturesDir(svc.DeckHome, second.ID)); err == nil {
		t.Fatalf("cleanup created a captures dir for the new session %q", second.ID)
	}
}

// TestCreateAgentReusingATombstonedNameCleansUpThatSessionsFiles proves
// the same post-commit cleanup on the agent create route, which has its
// own CreateSession call site and would otherwise silently keep the
// orphaned files.
func TestCreateAgentReusingATombstonedNameCleansUpThatSessionsFiles(t *testing.T) {
	svc, _, _, _ := newAgentTestService(t, nil, "agent-reuse-test")
	ctx := context.Background()
	first, err := svc.CreateAgent(ctx, AgentCreateInput{Name: "Shell: reuse", CWD: t.TempDir(), Agent: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	capturesDir, historyFile := seedSessionFiles(t, svc, first.ID)
	if err := svc.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateAgent(ctx, AgentCreateInput{Name: "Shell: reuse", CWD: t.TempDir(), Agent: "shell"})
	if err != nil {
		t.Fatalf("CreateAgent reusing a tombstoned name: %v, want success", err)
	}
	if second.ID == first.ID {
		t.Fatalf("reuse returned the old row: id %q", second.ID)
	}
	exists, err := svc.Store.SessionRowExists(ctx, first.ID)
	if err != nil || exists {
		t.Fatalf("tombstoned holder row after reuse: exists = %v, %v, want gone", exists, err)
	}
	if _, err := os.Stat(capturesDir); !os.IsNotExist(err) {
		t.Fatalf("captures dir after reuse: stat = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(historyFile); !os.IsNotExist(err) {
		t.Fatalf("history file after reuse: stat = %v, want IsNotExist", err)
	}
}

// TestRefusedCreateKeepsTheStillRestorableSessionsFiles is why the cleanup
// runs after the commit and not before: a create that the store REFUSES
// (here a live holder of the same name) must not have destroyed anything,
// and a tombstoned session whose name was never actually taken keeps its
// scrollback so `u` can still restore something meaningful.
func TestRefusedCreateKeepsTheStillRestorableSessionsFiles(t *testing.T) {
	svc := newTombstoneTestService(t)
	ctx := context.Background()
	live, err := svc.CreateShell(ctx, ShellCreateInput{Name: "live holder", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	liveCaptures, liveHistory := seedSessionFiles(t, svc, live.ID)

	tombstoned, err := svc.CreateShell(ctx, ShellCreateInput{Name: "kept holder", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	keptCaptures, keptHistory := seedSessionFiles(t, svc, tombstoned.ID)
	if err := svc.Delete(ctx, tombstoned); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateShell(ctx, ShellCreateInput{Name: "live holder", CWD: t.TempDir()}); err == nil {
		t.Fatal("CreateShell onto a live holder's name returned nil, want a refusal")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("refusal for a live holder = %v, want an already-exists error", err)
	}
	for _, path := range []string{liveCaptures, liveHistory, keptCaptures, keptHistory} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%q after a refused create: stat = %v, want untouched", path, err)
		}
	}

	// And the tombstoned holder is still restorable, files and all.
	if _, err := svc.Restore(ctx, tombstoned.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{keptCaptures, keptHistory} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%q after restore: stat = %v, want untouched", path, err)
		}
	}
}
