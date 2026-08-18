package service

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
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
	socket := "deck-reconcile-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	service := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}

	alpha := reconcileSession(t, db, "00000000-0000-4000-8000-000000000021", "alpha", cwd)
	beta := reconcileSession(t, db, "00000000-0000-4000-8000-000000000022", "beta", cwd)
	for _, session := range []store.Session{alpha, beta} {
		if _, err := service.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
			t.Fatalf("create live tmux session %q: %v", session.Name, err)
		}
	}

	const reconcileInterval = 250 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		err := service.RunReconciler(ctx, reconcileInterval)
		if err != nil {
			t.Errorf("reconcile loop exited early: %v", err)
		}
		done <- err
	}()
	if err := service.TMux.Kill(context.Background(), alpha.Slug); err != nil {
		t.Fatal(err)
	}
	// The disappearance must be visible within the configured cadence, not
	// merely eventually after an arbitrary multi-interval grace period.
	waitForStatus(t, db, alpha.ID, "stopped", reconcileInterval)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("reconcile loop: %v", err)
	}

	rows, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "stopped" || rows[0].StatusSource != "tmux" || rows[1].Status != "running" {
		t.Fatalf("reconciled rows = %#v", rows)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.session_gone'`, alpha.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("disappearance events = %d, %v", events, err)
	}
	if !auditContains(t, logger.Path(), alpha.ID, "tmux.session_gone") {
		t.Fatalf("audit lacks disappearance transition for %q", alpha.ID)
	}

	if err := exec.Command("tmux", "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill private tmux server: %v", err)
	}
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile killed server: %v", err)
	}
	waitForStatus(t, db, beta.ID, "stopped", 100*time.Millisecond)
	if err := exec.Command("tmux", "-L", socket, "list-sessions").Run(); err == nil {
		t.Fatal("reconciliation recreated the killed private tmux server")
	}
}

func reconcileSession(t *testing.T, db *store.Store, id, name, cwd string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{ID: id, Name: name, CWD: cwd, Agent: "shell", CapturedPath: "/bin", Status: "running", StatusSource: "user", StatusAt: 1, CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func waitForStatus(t *testing.T, db *store.Store, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows, err := db.ListSessions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == id && row.Status == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %q did not become %q within %s", id, want, timeout)
}

func auditContains(t *testing.T, path, sessionID, event string) bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"session_id":"`+sessionID+`"`) && strings.Contains(scanner.Text(), `"event":"`+event+`"`) {
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
