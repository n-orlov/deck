package service

import (
	"context"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlite "modernc.org/sqlite"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// wallClockSQLFunc is the SQL function the stopped-write triggers call. It
// returns the writer's wall clock in nanoseconds when the statement runs. It is
// registered before any test opens a store, so every connection has it.
const wallClockSQLFunc = "deck_test_wall_ns"

func init() {
	sqlite.MustRegisterScalarFunction(wallClockSQLFunc, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		return time.Now().UnixNano(), nil
	})
}

// recordStoppedWrites installs triggers that stamp every write of the stopped
// transition with the writer's wall clock: the status update to "stopped" and
// its tmux.session_gone event row. Both rows are written inside the same
// transaction, so a reader can only see the transition once both stamps exist.
// The returned function reports the latest stamp for a session, which is the
// last statement before the transaction commits.
func recordStoppedWrites(t *testing.T, db *store.Store) (stoppedAt func(id string) (time.Time, bool)) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE TABLE test_stopped_writes (session_id TEXT NOT NULL, at_ns INTEGER NOT NULL)`,
		`CREATE TRIGGER test_stopped_status AFTER UPDATE OF status ON sessions
			WHEN NEW.status = 'stopped' AND OLD.status IS NOT 'stopped'
			BEGIN INSERT INTO test_stopped_writes VALUES (NEW.id, ` + wallClockSQLFunc + `()); END`,
		`CREATE TRIGGER test_stopped_event AFTER INSERT ON events
			WHEN NEW.kind = 'tmux.session_gone'
			BEGIN INSERT INTO test_stopped_writes VALUES (NEW.session_id, ` + wallClockSQLFunc + `()); END`,
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Fatalf("install stopped-write recorder: %v", err)
		}
	}
	return func(id string) (time.Time, bool) {
		var ns *int64
		if err := db.DB().QueryRow(`SELECT max(at_ns) FROM test_stopped_writes WHERE session_id = ?`, id).Scan(&ns); err != nil {
			t.Fatalf("read stopped-write stamps: %v", err)
		}
		if ns == nil {
			return time.Time{}, false
		}
		return time.Unix(0, *ns), true
	}
}

// stoppedWrittenBy judges the stopped transition of id on when the writer
// recorded it, never on when this goroutine observed it. It waits up to stuck
// for the write to appear. That wait only guards a stuck loop. A write that
// appears at any point is then compared with bound, so a reader that runs late
// can neither fail an on-time write nor pass a late one.
func stoppedWrittenBy(stoppedAt func(string) (time.Time, bool), id string, bound time.Time, stuck time.Duration) error {
	deadline := time.Now().Add(stuck)
	for {
		if at, ok := stoppedAt(id); ok {
			if late := at.Sub(bound); late > 0 {
				return fmt.Errorf("session %q was written stopped %s after its bound", id, late)
			}
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("session %q was never written stopped (stuck-loop guard %s)", id, stuck)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestStoppedWriteIsJudgedOnWriteTimeNotObservation pins the rule the reconcile
// regression test relies on: the stopped transition meets its bound only if it
// was WRITTEN by the bound. The previous form observed the row and passed a
// write made after the deadline whenever the observer itself ran late. Each case
// writes the transition through the store at a controlled time and starts the
// observer at another time.
func TestStoppedWriteIsJudgedOnWriteTimeNotObservation(t *testing.T) {
	const bound = 60 * time.Millisecond
	for _, tc := range []struct {
		name          string
		writeAfter    time.Duration // when the stopped write runs, from the start
		status        string        // the status written
		observeAfter  time.Duration // when the observer first looks, from the start
		stuck         time.Duration
		wantViolation string // "" means the bound holds
	}{
		// A late write seen by an observer that also runs late (the finding's shape).
		{"late write, late observer", 140 * time.Millisecond, "stopped", 400 * time.Millisecond, time.Second, "after its bound"},
		// A late write seen by an observer polling from the start.
		{"late write, prompt observer", 140 * time.Millisecond, "stopped", 0, time.Second, "after its bound"},
		// An on-time write is not failed by an observer that first looks after the bound.
		{"on-time write, late observer", 5 * time.Millisecond, "stopped", 300 * time.Millisecond, time.Second, ""},
		// An on-time write seen promptly passes.
		{"on-time write, prompt observer", 5 * time.Millisecond, "stopped", 0, time.Second, ""},
		// A transition to some other status is not the stopped transition.
		{"other status only", 5 * time.Millisecond, "waiting", 0, 200 * time.Millisecond, "never written stopped"},
		// No write at all ends at the stuck-loop guard.
		{"no write", -1, "stopped", 0, 200 * time.Millisecond, "never written stopped"},
	} {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			db, id := stoppedWriteFixture(t)
			stoppedAt := recordStoppedWrites(t, db)
			start := time.Now()
			written := make(chan error, 1)
			if tc.writeAfter >= 0 {
				go func() {
					time.Sleep(tc.writeAfter)
					written <- db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
						SessionID: id, Status: tc.status, Reason: "test", Source: "tmux",
						At: 2, EventKind: eventKindFor(tc.status),
					})
				}()
			} else {
				written <- nil
			}
			time.Sleep(time.Until(start.Add(tc.observeAfter)))
			err := stoppedWrittenBy(stoppedAt, id, start.Add(bound), tc.stuck)
			if werr := <-written; werr != nil {
				t.Fatalf("write: %v", werr)
			}
			switch {
			case tc.wantViolation == "" && err != nil:
				t.Fatalf("on-time write judged a violation: %v", err)
			case tc.wantViolation != "" && (err == nil || !strings.Contains(err.Error(), tc.wantViolation)):
				t.Fatalf("got %v, want a violation containing %q", err, tc.wantViolation)
			}
		})
	}
}

// eventKindFor names the event a status write appends: only the stopped write
// is the tmux.session_gone transition.
func eventKindFor(status string) string {
	if status == "stopped" {
		return "tmux.session_gone"
	}
	return "tmux.shell_live"
}

func stoppedWriteFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	db, err := store.Open(config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	session := reconcileSession(t, db, "00000000-0000-4000-8000-000000000041", "delta", cwd, "shell", "running", "user")
	return db, session.ID
}
