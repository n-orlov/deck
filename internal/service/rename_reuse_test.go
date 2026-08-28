package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestRenameOntoATombstonedNameCleansUpThatSessionsFiles proves the
// service half of R77 on the RENAME route (SPEC §9.2), mirroring
// TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles for
// create: taking a deleted session's name via Rename reaps that session
// just like the store already does in RenameSession's own transaction,
// and a reap is not finished until deck's own per-session files (captures
// dir, §9.4 history file) go with it too. The store removes the row and
// its event inside RenameSession's transaction; the files are removed
// here, only after that commit -- and the renamed session's own files, if
// any, must survive untouched.
func TestRenameOntoATombstonedNameCleansUpThatSessionsFiles(t *testing.T) {
	svc := newArchiveTestService(t)
	ctx := context.Background()

	reaped, err := svc.CreateShell(ctx, ShellCreateInput{Name: "freed name", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	reapedCaptures, reapedHistory := seedSessionFiles(t, svc, reaped.ID)
	if err := svc.Delete(ctx, reaped); err != nil {
		t.Fatal(err)
	}

	subject, err := svc.CreateShell(ctx, ShellCreateInput{Name: "renamed subject", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	subjectCaptures, subjectHistory := seedSessionFiles(t, svc, subject.ID)

	renamed, err := svc.Rename(ctx, subject.ID, "freed name")
	if err != nil {
		t.Fatalf("rename onto a tombstoned name: %v, want success", err)
	}
	if renamed.Name != "freed name" {
		t.Fatalf("name = %q, want %q", renamed.Name, "freed name")
	}

	exists, err := svc.Store.SessionRowExists(ctx, reaped.ID)
	if err != nil || exists {
		t.Fatalf("tombstoned holder row after rename: exists = %v, %v, want gone", exists, err)
	}
	if _, err := os.Stat(reapedCaptures); !os.IsNotExist(err) {
		t.Fatalf("captures dir of the reaped holder after rename: stat = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(reapedHistory); !os.IsNotExist(err) {
		t.Fatalf("history file of the reaped holder after rename: stat = %v, want IsNotExist", err)
	}

	// The renamed session's own files were never touched by the cleanup.
	if _, err := os.Stat(subjectCaptures); err != nil {
		t.Fatalf("renamed session's captures dir after rename: stat = %v, want untouched", err)
	}
	if _, err := os.Stat(subjectHistory); err != nil {
		t.Fatalf("renamed session's history file after rename: stat = %v, want untouched", err)
	}
	if _, err := os.Stat(config.CapturesDir(svc.DeckHome, subject.ID)); err != nil {
		t.Fatalf("renamed session's captures dir (by id) after rename: stat = %v, want untouched", err)
	}
}

// TestRefusedRenameKeepsLiveAndArchivedHoldersFiles proves the cleanup
// only ever runs after RenameSession has committed: a rename the store
// REFUSES -- onto a live holder's name, or onto an archived holder's name
// (R78: archiving keeps a name reserved) -- must not touch either
// holder's files, exactly like a refused create already does not
// (TestRefusedCreateKeepsTheStillRestorableSessionsFiles).
func TestRefusedRenameKeepsLiveAndArchivedHoldersFiles(t *testing.T) {
	svc := newArchiveTestService(t)
	ctx := context.Background()

	live, err := svc.CreateShell(ctx, ShellCreateInput{Name: "live holder", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	liveCaptures, liveHistory := seedSessionFiles(t, svc, live.ID)

	archived, err := svc.CreateShell(ctx, ShellCreateInput{Name: "archived holder", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	archivedCaptures, archivedHistory := seedSessionFiles(t, svc, archived.ID)
	if err := svc.Store.ArchiveSession(ctx, archived.ID, svc.Clock.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}

	subject, err := svc.CreateShell(ctx, ShellCreateInput{Name: "rename subject", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Rename(ctx, subject.ID, "live holder"); err == nil {
		t.Fatal("rename onto a live holder's name returned nil, want a refusal")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("refusal for a live holder = %v, want an already-exists error", err)
	}
	if _, err := svc.Rename(ctx, subject.ID, "archived holder"); err == nil {
		t.Fatal("rename onto an archived holder's name returned nil, want a refusal")
	}

	for _, path := range []string{liveCaptures, liveHistory, archivedCaptures, archivedHistory} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%q after a refused rename: stat = %v, want untouched", path, err)
		}
	}

	unchanged, err := svc.Store.GetSession(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != "rename subject" {
		t.Fatalf("subject name after refused renames = %q, want unchanged %q", unchanged.Name, "rename subject")
	}
}
