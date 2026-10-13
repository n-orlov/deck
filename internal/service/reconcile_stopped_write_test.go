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

// stoppedTokenSQLFunc is the SQL function the stopped-transition trigger calls
// inside the writing transaction. It hands out a fresh token and records it on
// the database's recorder. The trigger stores the token in a row of
// test_stopped_marks, so the mark is part of the transaction: a statement,
// savepoint or transaction rollback that undoes the stopped assignment undoes
// its mark with it. The recorder only keeps when each token's transaction
// reached COMMIT; the judge reads which tokens actually committed from the
// table, so a token whose row never committed is never looked at.
const stoppedTokenSQLFunc = "deck_test_stopped_token"

// stoppedRecorders maps a state.db path to its *stoppedCommits recorder.
var stoppedRecorders sync.Map

func init() {
	sqlite.MustRegisterScalarFunction(stoppedTokenSQLFunc, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		path, _ := args[0].(string)
		if rec, ok := stoppedRecorders.Load(path); ok {
			return rec.(*stoppedCommits).issue(), nil
		}
		return int64(0), nil
	})
	// Every connection gets a commit hook that reports to the recorder of the
	// database it opened, if any. SQLite calls the commit hook only for a write
	// transaction, at its COMMIT, after every statement in it and after anything
	// the writer did between its last statement and COMMIT. SQLite runs one
	// write transaction per database at a time, so every token issued and not
	// yet stamped belongs to the transaction now committing, or to one that was
	// rolled back and whose rows therefore never committed.
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
		return nil
	})
}

// stoppedCommits records when the transaction holding each stopped-mark token
// reached COMMIT. Whether that token's mark committed at all is never kept
// here: it is the test_stopped_marks row, which only a committed transaction
// leaves behind.
type stoppedCommits struct {
	mu        sync.Mutex
	next      int64
	unstamped []int64
	stamped   map[int64]time.Time
}

func newStoppedCommits() *stoppedCommits {
	return &stoppedCommits{stamped: map[int64]time.Time{}}
}

func (r *stoppedCommits) issue() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	r.unstamped = append(r.unstamped, r.next)
	return r.next
}

func (r *stoppedCommits) commit(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, token := range r.unstamped {
		r.stamped[token] = at
	}
	r.unstamped = nil
}

func (r *stoppedCommits) committedAt(token int64) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.stamped[token]
	return at, ok
}

// stoppedView is what an independent connection reads in one statement: the
// session's status and the latest committed mark, if that mark is a stopped
// transition (a later move away from stopped leaves the latest mark a
// non-stopped one).
type stoppedView struct {
	status    string
	stopped   bool      // the latest committed mark is a stopped transition
	committed time.Time // when that transition's transaction committed
}

// stoppedTransition judges one database's stopped transitions: commits holds
// when each mark's transaction committed, and read reads a session's status and
// latest committed mark through a connection of its own, the way any other
// client of state.db sees it.
type stoppedTransition struct {
	commits *stoppedCommits
	read    func(id string) (stoppedView, error)
}

// recordStoppedWrites installs a mark table and triggers that add a row to it
// for every status write to "stopped" and every move away from it, and opens an
// independent reader on the same database file.
func recordStoppedWrites(t *testing.T, db *store.Store, path string) stoppedTransition {
	t.Helper()
	rec := newStoppedCommits()
	if _, loaded := stoppedRecorders.LoadOrStore(path, rec); loaded {
		t.Fatalf("a stopped-write recorder already watches %s", path)
	}
	t.Cleanup(func() { stoppedRecorders.Delete(path) })
	quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	for _, stmt := range []string{
		`CREATE TABLE test_stopped_marks (seq INTEGER PRIMARY KEY, session_id TEXT NOT NULL, stopped INTEGER NOT NULL, token INTEGER NOT NULL)`,
		`CREATE TRIGGER test_stopped_status AFTER UPDATE OF status ON sessions
			WHEN NEW.status = 'stopped' AND OLD.status IS NOT 'stopped'
			BEGIN INSERT INTO test_stopped_marks (session_id, stopped, token) VALUES (NEW.id, 1, ` + stoppedTokenSQLFunc + `(` + quoted + `)); END`,
		`CREATE TRIGGER test_stopped_left AFTER UPDATE OF status ON sessions
			WHEN OLD.status = 'stopped' AND NEW.status IS NOT 'stopped'
			BEGIN INSERT INTO test_stopped_marks (session_id, stopped, token) VALUES (NEW.id, 0, 0); END`,
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Fatalf("install stopped-write recorder: %v", err)
		}
	}
	reader, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open independent reader: %v", err)
	}
	reader.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reader.Close() })
	return stoppedTransition{commits: rec, read: func(id string) (stoppedView, error) {
		var view stoppedView
		var stopped, token sql.NullInt64
		err := reader.QueryRow(`SELECT s.status,
			(SELECT m.stopped FROM test_stopped_marks m WHERE m.session_id = s.id ORDER BY m.seq DESC LIMIT 1),
			(SELECT m.token FROM test_stopped_marks m WHERE m.session_id = s.id ORDER BY m.seq DESC LIMIT 1)
			FROM sessions s WHERE s.id = ?`, id).Scan(&view.status, &stopped, &token)
		if err != nil || stopped.Int64 != 1 {
			return view, err
		}
		at, ok := rec.committedAt(token.Int64)
		if !ok {
			return view, fmt.Errorf("stopped mark %d of session %q committed without passing the commit hook", token.Int64, id)
		}
		view.stopped, view.committed = true, at
		return view, nil
	}}
}

// stoppedCommittedBy judges the stopped transition of id on when the
// transaction that committed it COMMITTED, never on when this goroutine
// observed it, and never on a mark that did not commit with the stopped status.
// It waits up to stuck for a committed stopped mark to be the latest one. That
// wait only guards a stuck loop. The commit time of that mark is then compared
// with bound, so a reader that runs late can neither fail an on-time commit nor
// pass a late one. Once the bound has passed, an independent connection must
// read the status as stopped and the latest committed mark as one committed by
// the bound: a read that begins after the bound and sees anything else proves
// the transition the reader sees was not in place by the bound.
func stoppedCommittedBy(tr stoppedTransition, id string, bound time.Time, stuck time.Duration) error {
	deadline := time.Now().Add(stuck)
	for {
		view, err := tr.read(id)
		if err != nil {
			return fmt.Errorf("independent read of session %q: %w", id, err)
		}
		if view.stopped {
			if late := view.committed.Sub(bound); late > 0 {
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
	view, err := tr.read(id)
	if err != nil {
		return fmt.Errorf("independent read of session %q: %w", id, err)
	}
	if view.status != "stopped" || !view.stopped {
		return fmt.Errorf("an independent reader still saw session %q as %q %s after its bound", id, view.status, begun.Sub(bound))
	}
	if late := view.committed.Sub(bound); late > 0 {
		return fmt.Errorf("session %q was committed stopped %s after its bound", id, late)
	}
	return nil
}

// TestStoppedTransitionIsJudgedOnCommitNotObservation pins the rule the
// reconcile regression test relies on: the stopped transition meets its bound
// only if the transaction writing it COMMITTED by the bound and the stopped
// status is visible to another connection after it. An earlier form stamped the
// writing statements, so a transaction held open past the bound before its
// COMMIT passed. A later form kept an in-memory mark per stopped assignment,
// so an assignment undone by a savepoint or statement rollback still stood in
// for a stopped status committed after the bound; the mark is now a row of the
// writing transaction itself. Each case writes at controlled times and starts
// the observer at another time.
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
		{"stopped, then overwritten, late observer", []step{storeWrite("stopped"), sleep(5 * time.Millisecond), rawStatus("running")}, 100 * time.Millisecond, 200 * time.Millisecond, "never committed stopped"},
		// The same, overwritten while the observer is already waiting for the bound.
		{"stopped, then overwritten, prompt observer", []step{storeWrite("stopped"), sleep(30 * time.Millisecond), rawStatus("running")}, 0, time.Second, "independent reader still saw"},
		// Precursor marks that are not the stopped status: an event row alone,
		// or a stopped assignment undone inside its own transaction, commit at
		// once. The actual stopped status lands after the bound, so the
		// precursor's early commit must not stand in for it.
		{"event only, then late stopped", []step{eventOnly, sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		{"event only, then late stopped, late observer", []step{eventOnly, sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		{"overwritten inside transaction, then late stopped", []step{overwrittenInTx, sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		{"overwritten inside transaction, then late stopped, late observer", []step{overwrittenInTx, sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		// Stopped committed on time, reverted, and stopped again only after the bound.
		{"stopped, reverted, late stopped again", []step{storeWrite("stopped"), rawStatus("running"), sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		// The same precursors undone below the transaction: a savepoint rolled
		// back (then released), a stopped assignment in an inner savepoint
		// rolled back while the outer one is released, and a statement whose
		// stopped row is undone when a later row of the same statement fails.
		// Each transaction commits only its event, at once.
		{"savepoint rolled back, then late stopped", []step{savepointRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		{"savepoint rolled back, then late stopped, late observer", []step{savepointRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		{"inner savepoint rolled back, then late stopped", []step{innerSavepointRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		{"inner savepoint rolled back, then late stopped, late observer", []step{innerSavepointRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		{"statement rolled back, then late stopped", []step{statementRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, "after its bound"},
		{"statement rolled back, then late stopped, late observer", []step{statementRolledBack, sleep(140 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, "after its bound"},
		{"savepoint rolled back", []step{savepointRolledBack}, 0, 200 * time.Millisecond, "never committed stopped"},
		{"statement rolled back", []step{statementRolledBack}, 0, 200 * time.Millisecond, "never committed stopped"},
		// A stopped assignment that a savepoint rollback undoes and the same
		// transaction then makes again commits the stopped status on time.
		{"savepoint rolled back, stopped again in the same transaction", []step{savepointRolledBackThenStopped}, 0, time.Second, ""},
		// Another connection rolling back its own read transaction while the
		// stopped write is open does not touch that write's commit.
		{"foreign read rolled back during an on-time write", []step{heldWriteWithForeignRollback(20 * time.Millisecond)}, 0, time.Second, ""},
		{"foreign read rolled back during an on-time write, late observer", []step{heldWriteWithForeignRollback(20 * time.Millisecond)}, 300 * time.Millisecond, time.Second, ""},
		// A precursor alone never counts.
		{"event only", []step{eventOnly}, 0, 200 * time.Millisecond, "never committed stopped"},
		{"overwritten inside transaction", []step{overwrittenInTx}, 0, 200 * time.Millisecond, "never committed stopped"},
		// Precursors followed by the actual stopped status in time pass.
		{"event only, then on-time stopped", []step{eventOnly, sleep(5 * time.Millisecond), storeWrite("stopped")}, 300 * time.Millisecond, time.Second, ""},
		{"overwritten inside transaction, then on-time stopped", []step{overwrittenInTx, sleep(5 * time.Millisecond), storeWrite("stopped")}, 0, time.Second, ""},
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

// eventOnly commits the transition's event row without the stopped status.
func eventOnly(t *testing.T, db *store.Store, id string) {
	if _, err := db.DB().Exec(`INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`, id); err != nil {
		t.Errorf("insert event: %v", err)
	}
}

// overwrittenInTx assigns the stopped status and puts the old status back
// before the same transaction commits.
func overwrittenInTx(t *testing.T, db *store.Store, id string) {
	tx, err := db.DB().Begin()
	if err != nil {
		t.Errorf("begin: %v", err)
		return
	}
	if _, err := tx.Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, id); err != nil {
		t.Errorf("update status: %v", err)
	}
	if _, err := tx.Exec(`UPDATE sessions SET status = 'running' WHERE id = ?`, id); err != nil {
		t.Errorf("restore status: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("commit: %v", err)
	}
}

// savepointRolledBack assigns the stopped status inside a savepoint, rolls the
// savepoint back and releases it, and commits only the event row.
func savepointRolledBack(t *testing.T, db *store.Store, id string) {
	inTx(t, db, func(tx *sql.Tx) {
		execAll(t, tx, id,
			`SAVEPOINT stop`,
			`UPDATE sessions SET status = 'stopped' WHERE id = ?`,
			`ROLLBACK TO stop`,
			`RELEASE stop`,
			`INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`)
	})
}

// innerSavepointRolledBack assigns the stopped status in a savepoint nested in
// another, rolls back only the inner one, releases the outer one and commits.
func innerSavepointRolledBack(t *testing.T, db *store.Store, id string) {
	inTx(t, db, func(tx *sql.Tx) {
		execAll(t, tx, id,
			`SAVEPOINT outer_sp`,
			`INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`,
			`SAVEPOINT inner_sp`,
			`UPDATE sessions SET status = 'stopped' WHERE id = ?`,
			`ROLLBACK TO inner_sp`,
			`RELEASE outer_sp`)
	})
}

// savepointRolledBackThenStopped undoes a stopped assignment with a savepoint
// rollback, then assigns it again in the same transaction and commits at once.
func savepointRolledBackThenStopped(t *testing.T, db *store.Store, id string) {
	inTx(t, db, func(tx *sql.Tx) {
		execAll(t, tx, id,
			`SAVEPOINT stop`,
			`UPDATE sessions SET status = 'stopped' WHERE id = ?`,
			`ROLLBACK TO stop`,
			`RELEASE stop`,
			`UPDATE sessions SET status = 'stopped' WHERE id = ?`,
			`INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`)
	})
}

// statementRolledBack runs one UPDATE that stops the session and then fails on
// a second row of the same statement (a duplicate name), so SQLite undoes the
// whole statement, the stopped row included, while the transaction goes on to
// commit the event row.
func statementRolledBack(t *testing.T, db *store.Store, id string) {
	other, err := db.CreateSession(context.Background(), store.CreateSessionInput{ID: "00000000-0000-4000-8000-000000000042", Name: "epsilon", CWD: t.TempDir(), Agent: "shell", CapturedPath: "/bin", Status: "running", StatusSource: "user", StatusAt: 1, CreatedAt: 1})
	if err != nil {
		t.Errorf("second session: %v", err)
		return
	}
	inTx(t, db, func(tx *sql.Tx) {
		_, err := tx.Exec(`UPDATE sessions SET status = 'stopped',
			name = CASE WHEN id = ?1 THEN name ELSE (SELECT name FROM sessions WHERE id = ?1) END
			WHERE id IN (?1, ?2)`, id, other.ID)
		if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
			t.Errorf("the two-row stop was meant to fail on its second row, got %v", err)
		}
		execAll(t, tx, id, `INSERT INTO events (session_id, kind, at) VALUES (?, 'tmux.session_gone', 2)`)
	})
	var status string
	if err := db.DB().QueryRow(`SELECT status FROM sessions WHERE id = ?`, id).Scan(&status); err != nil || status != "running" {
		t.Errorf("after the failed statement the status is %q, %v; want running", status, err)
	}
}

// heldWriteWithForeignRollback writes the stopped status and its event, and
// while that transaction is open another connection begins a read transaction
// and rolls it back. The write commits after hold.
func heldWriteWithForeignRollback(hold time.Duration) func(*testing.T, *store.Store, string) {
	return func(t *testing.T, db *store.Store, id string) {
		path := ""
		if err := db.DB().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
			t.Errorf("database path: %v", err)
			return
		}
		foreign, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Errorf("open foreign connection: %v", err)
			return
		}
		defer foreign.Close()
		inTx(t, db, func(tx *sql.Tx) {
			stoppedStatements(t, tx, id)
			ftx, err := foreign.Begin()
			if err != nil {
				t.Errorf("foreign begin: %v", err)
				return
			}
			var n int
			if err := ftx.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil {
				t.Errorf("foreign read: %v", err)
			}
			if err := ftx.Rollback(); err != nil {
				t.Errorf("foreign rollback: %v", err)
			}
			time.Sleep(hold)
		})
	}
}

// inTx runs body in one transaction and commits it.
func inTx(t *testing.T, db *store.Store, body func(*sql.Tx)) {
	tx, err := db.DB().Begin()
	if err != nil {
		t.Errorf("begin: %v", err)
		return
	}
	body(tx)
	if err := tx.Commit(); err != nil {
		t.Errorf("commit: %v", err)
	}
}

// execAll runs each statement in tx, binding the session id where a statement
// takes one.
func execAll(t *testing.T, tx *sql.Tx, id string, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		var err error
		if strings.Contains(stmt, "?") {
			_, err = tx.Exec(stmt, id)
		} else {
			_, err = tx.Exec(stmt)
		}
		if err != nil {
			t.Errorf("%s: %v", stmt, err)
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
