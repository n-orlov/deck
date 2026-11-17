package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R416 (F4): a status write bounded by its context's deadline gives up on a
// held write lock when that deadline passes, not after busy_timeout (5 s), and
// leaves the connection's busy_timeout as it found it.
func TestUpdateSessionStatusGivesUpOnAHeldWriteLockAtItsContextDeadline(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	other, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	if _, err := s.CreateSession(context.Background(), CreateSessionInput{
		ID: "row", Name: "row", CWD: home, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	lock, err := other.DB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = lock.Rollback()
		}
	}
	defer release()

	update := StatusUpdateInput{SessionID: "row", Status: "stopped", Source: "tmux", At: 2000, EventKind: "tmux.session_gone", Reason: "gone"}
	const bound = 300 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	started := time.Now()
	err = s.UpdateSessionStatus(ctx, update)
	elapsed := time.Since(started)
	if err == nil || !strings.Contains(err.Error(), "begin session status update") {
		t.Fatalf("update behind a held write lock = %v, want a begin session status update failure", err)
	}
	// Read by the clock: SQLite's busy wait can end the instant the deadline
	// passes, before the context's timer has marked the context done.
	if elapsed < bound {
		t.Fatalf("the update gave up after %s, before its context's %s deadline", elapsed, bound)
	}
	if limit := bound + time.Second; elapsed > limit {
		t.Fatalf("update took %s behind the held lock, want it cut off near its %s deadline (< %s), not at busy_timeout", elapsed, bound, limit)
	}

	release()
	var timeout int
	if err := s.DB().QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil || timeout != busyTimeoutMillis {
		t.Fatalf("busy_timeout after the bounded update = %d, %v; want %d", timeout, err, busyTimeoutMillis)
	}
	if err := s.UpdateSessionStatus(context.Background(), update); err != nil {
		t.Fatalf("update after the lock was released: %v", err)
	}
}
