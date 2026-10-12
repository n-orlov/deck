package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// lockedRollbackDB builds a rollback-journal database whose meta table holds
// version, then returns a function that releases an EXCLUSIVE lock taken on it.
// A reader arriving while that lock is held gets SQLITE_BUSY at once unless its
// connection already carries a busy timeout.
func lockedRollbackDB(t *testing.T, version int) (home, path string, release func()) {
	t.Helper()
	home = t.TempDir()
	path = filepath.Join(home, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	setup := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER)`,
		`INSERT INTO meta (key, version) VALUES ('schema_version', ` + itoa(version) + `)`,
	}
	if version < 0 { // the supported pre-v1 shape: no meta table at all
		setup = []string{`CREATE TABLE unrelated (x INTEGER)`}
	}
	for _, q := range setup {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	return home, path, func() {
		if _, err := conn.ExecContext(context.Background(), `COMMIT`); err != nil {
			t.Error(err)
		}
		_ = conn.Close()
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func releaseAfter(d time.Duration, release func()) {
	go func() {
		time.Sleep(d)
		release()
	}()
}

// The first read of a hook's store open already honours the contention budget:
// a writer holding the lock for a moment does not fail the open.
func TestOpenPathWaitsOutContentionOnTheFirstSchemaRead(t *testing.T) {
	home, path, release := lockedRollbackDB(t, SchemaVersion)
	releaseAfter(300*time.Millisecond, release)
	start := time.Now()
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("OpenPath under brief contention = %v; want it to wait the lock out", err)
	}
	defer s.Close()
	if time.Since(start) < 200*time.Millisecond {
		t.Fatalf("OpenPath returned in %v, before the holder released", time.Since(start))
	}
}

// Same rule, other shape: a pre-v1 database with no meta table is read, then
// migrated, after the lock clears.
func TestOpenPathMigratesAPreV1DatabaseAfterContentionOnTheFirstRead(t *testing.T) {
	home, path, release := lockedRollbackDB(t, -1)
	releaseAfter(300*time.Millisecond, release)
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("OpenPath under brief contention = %v; want it to wait the lock out", err)
	}
	defer s.Close()
	var version int
	if err := s.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("schema version = %d, %v; want %d", version, err, SchemaVersion)
	}
}

// A newer schema met during contention is still refused, and still untouched.
func TestOpenPathRefusesNewerSchemaAfterContentionWithoutWriting(t *testing.T) {
	home, path, release := lockedRollbackDB(t, SchemaVersion+1)
	releaseAfter(300*time.Millisecond, release)
	_, err := OpenPath(home, path)
	var newer *NewerSchemaError
	if !errors.As(err, &newer) || newer.DB != SchemaVersion+1 {
		t.Fatalf("err = %v; want NewerSchemaError for %d", err, SchemaVersion+1)
	}
	for _, sibling := range []string{path + "-wal", path + "-shm"} {
		if _, statErr := os.Stat(sibling); statErr == nil {
			t.Fatalf("%s exists: the refusal wrote to the database", sibling)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("journal_mode = %q, %v; want the untouched delete mode", mode, err)
	}
}

// Contention that outlasts the existing budget still fails rather than hang.
func TestOpenPathStillFailsWhenContentionOutlastsTheBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the full busy timeout")
	}
	home, path, release := lockedRollbackDB(t, SchemaVersion)
	defer release()
	start := time.Now()
	s, err := OpenPath(home, path)
	if err == nil {
		_ = s.Close()
		t.Fatal("OpenPath succeeded while the database stayed exclusively locked")
	}
	if elapsed := time.Since(start); elapsed < time.Duration(busyTimeoutMillis)*time.Millisecond-500*time.Millisecond || elapsed > 15*time.Second {
		t.Fatalf("gave up after %v; want about the %d ms budget", elapsed, busyTimeoutMillis)
	}
}

func TestImmediateTxDSNCarriesBusyTimeoutWithAndWithoutAQuery(t *testing.T) {
	for in, want := range map[string]string{
		"/x/state.db":     "/x/state.db?_txlock=immediate&_pragma=busy_timeout(5000)",
		"/x/state.db?a=b": "/x/state.db?a=b&_txlock=immediate&_pragma=busy_timeout(5000)",
	} {
		if got := immediateTxDSN(in); got != want {
			t.Errorf("immediateTxDSN(%q) = %q; want %q", in, got, want)
		}
	}
}
