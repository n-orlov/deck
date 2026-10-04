package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestDeleteRestoreReapRefuseWhatTheyCannotDo: the tombstone lifecycle names
// its missing dependencies and blank ids, reports a refused kill without
// tombstoning the row, and reports a store that can no longer commit.
func TestDeleteRestoreReapRefuseWhatTheyCannotDo(t *testing.T) {
	ctx := context.Background()

	t.Run("unwired service", func(t *testing.T) {
		var zero Service
		_, err := zero.Delete(ctx, store.Session{ID: "id"})
		wantErr(t, "Delete", err, "requires store, audit logger, and clock")
		_, err = zero.Restore(ctx, "id")
		wantErr(t, "Restore", err, "requires store, audit logger, and clock")
		wantErr(t, "Reap", zero.Reap(ctx, "id"), "requires store and clock")
	})
	t.Run("blank ids", func(t *testing.T) {
		svc := preconditionService(t, tmuxAnswersEverything)
		_, err := svc.Delete(ctx, store.Session{})
		wantErr(t, "Delete", err, "durable session id")
		_, err = svc.Restore(ctx, "")
		wantErr(t, "Restore", err, "durable session id")
		wantErr(t, "Reap", svc.Reap(ctx, ""), "durable session id")
	})
	t.Run("live row without a slug", func(t *testing.T) {
		svc := preconditionService(t, tmuxAnswersEverything)
		_, err := svc.Delete(ctx, store.Session{ID: "x", Name: "slugless", Status: "running"})
		wantErr(t, "Delete", err, "requires a durable slug to kill a live pane")
	})
	t.Run("kill refused leaves the row undeleted", func(t *testing.T) {
		svc := preconditionService(t, tmuxRefusesEverything)
		session := seedSession(t, svc, "live delete", "claude", "running")
		_, err := svc.Delete(ctx, session)
		wantErr(t, "Delete", err, `kill tmux session "live delete"`)
		got, err := svc.Store.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.DeletedAt != 0 {
			t.Fatalf("a delete whose kill failed still tombstoned the row (deleted_at=%d)", got.DeletedAt)
		}
	})
	t.Run("store closed", func(t *testing.T) {
		svc := preconditionService(t, tmuxAnswersEverything)
		session := seedSession(t, svc, "closed delete", "claude", "running")
		if err := svc.Store.Close(); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Delete(ctx, session)
		wantErr(t, "Delete live", err, `record killed session "closed delete"`)
		session.Status = "stopped"
		_, err = svc.Delete(ctx, session)
		wantErr(t, "Delete stopped", err, `tombstone session "closed delete"`)
		_, err = svc.Restore(ctx, session.ID)
		wantErr(t, "Restore", err, "restore session")
		wantErr(t, "Reap", svc.Reap(ctx, session.ID), "reap session")
	})
}

// TestReapRemovesTheSessionFilesOnlyWhenADeckHomeIsConfigured: a tombstoned
// session's captures directory and history file are removed on reap, and a
// service without a deck home reaps the row without touching the filesystem.
func TestReapRemovesTheSessionFilesOnlyWhenADeckHomeIsConfigured(t *testing.T) {
	ctx := context.Background()

	withoutHome := preconditionService(t, tmuxAnswersEverything)
	withoutHome.DeckHome = ""
	stoppedA := seedSession(t, withoutHome, "reap without home", "claude", "stopped")
	if _, err := withoutHome.Delete(ctx, stoppedA); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := withoutHome.Reap(ctx, stoppedA.ID); err != nil {
		t.Fatalf("reap without a deck home: %v", err)
	}
	if _, err := withoutHome.Store.GetSession(ctx, stoppedA.ID); err == nil {
		t.Fatalf("reaped row is still readable")
	}

	svc := preconditionService(t, tmuxAnswersEverything)
	stoppedB := seedSession(t, svc, "reap with home", "claude", "stopped")
	if _, err := svc.Delete(ctx, stoppedB); err != nil {
		t.Fatalf("delete: %v", err)
	}
	captures := config.CapturesDir(svc.DeckHome, stoppedB.ID)
	history := config.HistoryFile(svc.DeckHome, stoppedB.ID)
	if err := os.MkdirAll(captures, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(captures, "pane.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(history), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reap(ctx, stoppedB.ID); err != nil {
		t.Fatalf("reap: %v", err)
	}
	for _, gone := range []string{captures, history} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s survived the reap (stat err = %v)", gone, err)
		}
	}
	if _, err := os.Stat(svc.DeckHome); err != nil {
		t.Fatalf("reap removed more than the session's own files: %v", err)
	}
}

// TestInjectEnvRefusals: injection is shell-only and needs a live pane; each
// refusal names the session and the way forward, and tmux failures while
// probing the pane are reported rather than read as "not live".
func TestInjectEnvRefusals(t *testing.T) {
	ctx := context.Background()
	var zero Service
	_, _, err := zero.InjectEnv(ctx, "id")
	wantErr(t, "unwired", err, "requires a store and clock")

	svc := preconditionService(t, tmuxAnswersEverything)
	_, _, err = svc.InjectEnv(ctx, "")
	wantErr(t, "blank id", err, "session id is required")
	_, _, err = svc.InjectEnv(ctx, "missing")
	wantErr(t, "unknown id", err, `get session "missing"`)

	agentRow := seedSession(t, svc, "an agent", "claude", "running")
	_, _, err = svc.InjectEnv(ctx, agentRow.ID)
	wantErr(t, "agent row", err, "inject-instead only applies to shell sessions (use R to restart claude)")

	broken := preconditionService(t, tmuxRefusesEverything)
	shell := seedSession(t, broken, "a shell", "shell", "running")
	_, _, err = broken.InjectEnv(ctx, shell.ID)
	wantErr(t, "pane probe failing", err, `check live pane for session "a shell"`)
}

// TestRestartAndResumeRefusals covers the guard ladder both relaunchers share:
// unwired service, blank/unknown id, a tmux that cannot be probed, an
// unregistered agent kind, and a restart whose old pane will not die.
func TestRestartAndResumeRefusals(t *testing.T) {
	ctx := context.Background()
	var zero Service
	_, _, err := zero.Restart(ctx, "id")
	wantErr(t, "Restart unwired", err, "restart requires store, audit logger, clock, and adapter registry")
	_, _, err = zero.Resume(ctx, "id")
	wantErr(t, "Resume unwired", err, "resume requires store, audit logger, clock, and adapter registry")

	svc := preconditionService(t, tmuxAnswersEverything)
	_, _, err = svc.Restart(ctx, "")
	wantErr(t, "Restart blank", err, "session id is required")
	_, _, err = svc.Resume(ctx, "")
	wantErr(t, "Resume blank", err, "session id is required")
	_, _, err = svc.Restart(ctx, "missing")
	wantErr(t, "Restart unknown", err, `get session "missing"`)
	_, _, err = svc.Resume(ctx, "missing")
	wantErr(t, "Resume unknown", err, `get session "missing"`)

	ghost := seedSession(t, svc, "ghost kind", "ghost", "stopped")
	got, outcome, err := svc.Resume(ctx, ghost.ID)
	wantErr(t, "Resume unknown kind", err, `unknown agent kind "ghost"`)
	if outcome != ResumeStartingElsewhere || got.Status != "stopped" {
		t.Fatalf("an unresumable kind changed state: outcome=%v status=%q", outcome, got.Status)
	}

	blind := preconditionService(t, tmuxRefusesEverything)
	row := seedSession(t, blind, "blind", "claude", "running")
	_, outcome, err = blind.Resume(ctx, row.ID)
	wantErr(t, "Resume probe", err, `check for an already-running tmux session for "blind"`)
	if outcome != ResumeStartingElsewhere {
		t.Fatalf("outcome = %v, want ResumeStartingElsewhere", outcome)
	}
	_, _, err = blind.Restart(ctx, row.ID)
	wantErr(t, "Restart probe", err, `check live pane for session "blind"`)

	stubborn := preconditionService(t, tmuxLiveButRefusesWrites)
	live := seedSession(t, stubborn, "stubborn", "claude", "running")
	_, _, err = stubborn.Restart(ctx, live.ID)
	wantErr(t, "Restart kill", err, `kill tmux session "stubborn" for restart`)
}
