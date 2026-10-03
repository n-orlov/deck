package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	sqlite "modernc.org/sqlite"
)

// TestStoreKeepsTheWALAcrossClose: closing the last connection must leave
// state.db-wal on disk, so the next short-lived writer (a `deck _hook` with no
// TUI open) appends to it instead of starting a new WAL, whose header SQLite
// fsyncs before the first frame (SPEC §3.1's 20 ms hook write budget). The
// row must still be readable from state.db by a fresh Store afterwards.
func TestStoreKeepsTheWALAcrossClose(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.CreateSession(ctx, CreateSessionInput{
		ID: "kept", Name: "kept", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + "-wal")
	if err != nil {
		t.Fatalf("state.db-wal after the last close: %v (the WAL was deleted, so the next writer pays a header fsync)", err)
	}
	if info.Size() == 0 {
		t.Fatal("state.db-wal was truncated to zero on close; an empty WAL still costs the next writer a header fsync")
	}

	s, err = OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.GetSession(ctx, "kept"); err != nil {
		t.Fatalf("row written before the close is not readable after reopening: %v", err)
	}
}

// TestEveryConnectionInTheProcessKeepsTheWAL: the persistence is a driver-wide
// connection hook, not a step in OpenPath, because any connection that closes
// last without it deletes the WAL. A plain database/sql connection in the same
// process (as the features fixtures open) must report the mode on.
func TestEveryConnectionInTheProcessKeepsTheWAL(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mode := -2
	if err := conn.Raw(func(driverConn any) error {
		var err error
		mode, err = driverConn.(sqlite.FileControl).FileControlPersistWAL("main", -1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if mode != 1 {
		t.Fatalf("a plain connection's persistent-WAL mode = %d, want 1", mode)
	}
}

// TestANewDatabaseAndItsWALSiblingsAre0600: SQLite gives a new -wal and -shm
// the main file's mode at creation, so state.db must be 0600 before
// journal_mode=WAL runs. Otherwise the persistent WAL, which holds session env
// values, stays on disk world-readable (SPEC section 6.4).
func TestANewDatabaseAndItsWALSiblingsAre0600(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(context.Background(), CreateSessionInput{
		ID: "mode", Name: "mode", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("%s mode = %#o, want 0600", filepath.Base(p), mode)
		}
	}
}
