package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sqlite "modernc.org/sqlite"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// stoppedMarkSQLFunc is the SQL function the stopped-transition triggers call
// inside the writing transaction. It only marks the session as pending on the
// database's recorder. The transition counts as written when the transaction
// that holds the mark commits, which the commit hook below records.
const stoppedMarkSQLFunc = "deck_test_mark_stopped"

// stoppedRecorders maps a state.db path to its *stoppedCommits recorder.
var stoppedRecorders sync.Map

func init() {
	sqlite.MustRegisterScalarFunction(stoppedMarkSQLFunc, 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		path, _ := args[0].(string)
		id, _ := args[1].(string)
		if rec, ok := stoppedRecorders.Load(path); ok {
			rec.(*stoppedCommits).mark(id)
		}
		return nil, nil
	})
	// Every connection gets a commit hook and a rollback hook that report to the
	// recorder of the database it opened, if any. SQLite calls the commit hook
	// only for a write transaction, at its COMMIT, after every statement in it
	// and after anything the writer did between its last statement and COMMIT.
	// SQLite runs one write transaction per database at a time, so the next
	// commit or rollback on that database ends the transaction that set a mark.
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
		hooks, ok := conn.(sqlite.HookRegisterer)
		if !ok {
			return nil
		}
		path, _, _ := strings.Cut(dsn, "?")
		hooks.RegisterCommitHook(func() int32 {
			if rec, ok := stoppedRecorders.Load(path); ok {
				rec.(*stoppedCommits).commit(time.Now())
			}
			return 0
		})
		hooks.RegisterRollbackHook(func() {
			if rec, ok := stoppedRecorders.Load(path); ok {
				rec.(*stoppedCommits).rollback()
			}
		})
		return nil
	})
}

// stoppedCommits records, per session, when the transaction that wrote its
// stopped transition committed.
type stoppedCommits struct {
	mu        sync.Mutex
	pending   map[string]bool
	committed map[string]time.Time
}

func (r *stoppedCommits) mark(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending[id] = true
}

func (r *stoppedCommits) commit(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.pending {
		if _, done := r.committed[id]; !done {
			r.committed[id] = at
		}
	}
	r.pending = map[string]bool{}
}

func (r *stoppedCommits) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = map[string]bool{}
}

func (r *stoppedCommits) committedAt(id string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.committed[id]
	return at, ok
}

// stoppedTransition judges one database's stopped transitions: commits holds
// when each committed, and status reads a session's status through a
// connection of its own, the way any other client of state.db sees it.
type stoppedTransition struct {
	commits *stoppedCommits
	status  func(id string) (string, error)
}

// recordStoppedWrites installs triggers that mark every write of the stopped
// transition (the status update to "stopped" and its tmux.session_gone event
// row) and opens an independent reader on the same database file.
func recordStoppedWrites(t *testing.T, db *store.Store, path string) stoppedTransition {
	t.Helper()
	rec := &stoppedCommits{pending: map[string]bool{}, committed: map[string]time.Time{}}
	if _, loaded := stoppedRecorders.LoadOrStore(path, rec); loaded {
		t.Fatalf("a stopped-write recorder already watches %s", path)
	}
	t.Cleanup(func() { stoppedRecorders.Delete(path) })
	quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	for _, stmt := range []string{
		`CREATE TRIGGER test_stopped_status AFTER UPDATE OF status ON sessions
			WHEN NEW.status = 'stopped' AND OLD.status IS NOT 'stopped'
			BEGIN SELECT ` + stoppedMarkSQLFunc + `(` + quoted + `, NEW.id); END`,
		`CREATE TRIGGER test_stopped_event AFTER INSERT ON events
			WHEN NEW.kind = 'tmux.session_gone'
			BEGIN SELECT ` + stoppedMarkSQLFunc + `(` + quoted + `, NEW.session_id); END`,
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Fatalf("install stopped-write recorder: %v", err)
		}
	}
	reader, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open independent reader: %v", err)
	}
	reader.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reader.Close() })
	return stoppedTransition{commits: rec, status: func(id string) (string, error) {
		var status string
		err := reader.QueryRow(`SELECT status FROM sessions WHERE id = ?`, id).Scan(&status)
		return status, err
	}}
}

// stoppedCommittedBy judges the stopped transition of id on when its
// transaction committed, never on when this goroutine observed it. It waits up
// to stuck for the commit to happen. That wait only guards a stuck loop. A
// commit seen at any point is then compared with bound, so a reader that runs
// late can neither fail an on-time commit nor pass a late one. Once the bound
// has passed, an independent connection must read the status as stopped: a
// read that begins after the bound and still sees another status proves the
// transition was not visible by the bound, whatever the commit hook recorded.
func stoppedCommittedBy(tr stoppedTransition, id string, bound time.Time, stuck time.Duration) error {
	deadline := time.Now().Add(stuck)
	for {
		if at, ok := tr.commits.committedAt(id); ok {
			if late := at.Sub(bound); late > 0 {
				return fmt.Errorf("session %q was committed stopped %s after its bound", id, late)
			}
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("session %q was never committed stopped (stuck-loop guard %s)", id, stuck)
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(time.Until(bound))
	begun := time.Now()
	status, err := tr.status(id)
	if err != nil {
		return fmt.Errorf("independent read of session %q: %w", id, err)
	}
	if status != "stopped" {
		return fmt.Errorf("an independent reader still saw session %q as %q %s after its bound", id, status, begun.Sub(bound))
	}
	return nil
}

// TestStoppedTransitionIsJudgedOnCommitNotObservation pins the rule the
// reconcile regression test relies on: the stopped transition meets its bound
// only if the transaction writing it COMMITTED by the bound and the stopped
// status is visible to another connection after it. An earlier form stamped the
// writing statements, so a transaction held open past the bound before its
// COMMIT passed. Each case writes at controlled times and starts the observer
// at another time.
func TestStoppedTransitionIsJudgedOnCommitNotObservation(t *testing.T) {
	const bound = 60 * time.Millisecond
	type step func(t *testing.T, db *store.Store, id string)
	sleep := func(d time.Duration) step { return func(*testing.T, *store.Store, string) { time.Sleep(d) } }
	for _, tc := range []struct {
		name          string
		write         []step        // run in order from the start, beside the observer
		observeAfter  time.Duration // when the observer first looks, from the start
		stuck         time.Duration
		wantViolation string // "" means the bound holds
	}{
		// A late write seen by an observer that also runs late.
		{"late write, late observer", []step{sleep(140 * time.Millisecond), storeWrite("stopped")}, 400 * time.Millisecond, time.Second, "after its bound"},
		// A late write seen by an observer polling from the start.
		{"late write, prompt observer", []step{sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		// The statements run at once but COMMIT runs after the bound (the
		// rejected form passed this: its stamps predated the commit).
		{"on-time statements, late commit", []step{heldWrite(140 * time.Millisecond)}, 0, time.Second, "committed stopped"},
		// The same, with an observer that only looks after the late commit.
		{"on-time statements, late commit, late observer", []step{heldWrite(140 * time.Millisecond)}, 300 * time.Millisecond, time.Second, "committed stopped"},
		// A stopped write that rolls back never committed the transition.
		{"rolled back", []step{rolledBackWrite}, 0, 200 * time.Millisecond, "never committed stopped"},
		// A committed stopped status overwritten before the bound is not
		// visible at the bound.
		{"stopped, then overwritten", []step{storeWrite("stopped"), sleep(5 * time.Millisecond), rawStatus("running")}, 0, time.Second, "independent reader still saw"},
		// An on-time write is not failed by an observer that first looks after the bound.
		{"on-time write, late observer", []step{sleep(5 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, ""},
		// An on-time write seen promptly passes.
		{"on-time write, prompt observer", []step{sleep(5 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, ""},
		// A transition to some other status is not the stopped transition.
		{"other status only", []step{sleep(5 * time.Millisecond), storeWrite("waiting")}, 0, 200 * time.Millisecond, "never committed stopped"},
		// No write at all ends at the stuck-loop guard.
		{"no write", nil, 0, 200 * time.Millisecond, "never committed stopped"},
	} {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			db, path, id := stoppedWriteFixture(t)
			tr := recordStoppedWrites(t, db, path)
			start := time.Now()
			written := make(chan struct{})
			go func() {
				defer close(written)
				for _, s := range tc.write {
					s(t, db, id)
				}
			}()
			time.Sleep(time.Until(start.Add(tc.observeAfter)))
			err := stoppedCommittedBy(tr, id, start.Add(bound), tc.stuck)
			<-written
			switch {
			case tc.wantViolation == "" && err != nil:
				t.Fatalf("on-time transition judged a violation: %v", err)
			case tc.wantViolation != "" && (err == nil || !strings.Contains(err.Error(), tc.wantViolation)):
				t.Fatalf("got %v, want a violation containing %q", err, tc.wantViolation)
			}
		})
	}
}

// storeWrite writes a status through the store, the way the reconciler does.
func storeWrite(status string) func(*testing.T, *store.Store, string) {
	return func(t *testing.T, db *store.Store, id string) {
		if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
			SessionID: id, Status: status, Reason: "test", Source: "tmux",
			At: 2, EventKind: eventKindFor(status),
		}); err != nil {
			t.Errorf("write %s: %v", status, err)
		}
	}
}

// heldWrite writes the stopped status and its event at once and holds the
// transaction open for hold before COMMIT.
func heldWrite(hold time.Duration) func(*testing.T, *store.Store, string) {
	return func(t *testing.T, db *store.Store, id string) {
		tx, err := db.DB().Begin()
		if err != nil {
			t.Errorf("begin: %v", err)
			return
		}
		stoppedStatements(t, tx, id)
		time.Sleep(hold)
		if err := tx.Commit(); err != nil {
			t.Errorf("commit: %v", err)
		}
	}
}

// rolledBackWrite writes the stopped status and its event, then rolls back.
func rolledBackWrite(t *testing.T, db *store.Store, id string) {
	tx, err := db.DB().Begin()
	if err != nil {
		t.Errorf("begin: %v", err)
		return
	}
	stoppedStatements(t, tx, id)
	if err := tx.Rollback(); err != nil {
		t.Errorf("rollback: %v", err)
	}
}

func stoppedStatements(t *testing.T, tx *sql.Tx, id string) {
	if _, err := tx.Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, id); err != nil {
		t.Errorf("update status: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`, id); err != nil {
		t.Errorf("insert event: %v", err)
	}
}

// rawStatus overwrites the status column alone.
func rawStatus(status string) func(*testing.T, *store.Store, string) {
	return func(t *testing.T, db *store.Store, id string) {
		if _, err := db.DB().Exec(`UPDATE sessions SET status = ? WHERE id = ?`, status, id); err != nil {
			t.Errorf("overwrite status: %v", err)
		}
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

func stoppedWriteFixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	session := reconcileSession(t, db, "00000000-0000-4000-8000-000000000041", "delta", cwd, "shell", "running", "user")
	return db, paths.StateDB, session.ID
}
