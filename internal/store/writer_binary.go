package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

// writerBinaryKey is the meta row that records which deck binary last
// migrated or opened the database for writing (SPEC section 3.1, R204). The
// path lives in the existing meta table's value column: SQLite does not
// enforce the column's INTEGER affinity on text that is not a number, so the
// row is an additive write and not a schema change (SchemaVersion is
// untouched, and a binary that predates it ignores the row).
const writerBinaryKey = "writer_binary"

// executablePath resolves the running binary; tests substitute it.
var executablePath = os.Executable

// WriterBinary returns the absolute path of the deck binary recorded as the
// database's last writer, or "" when none is recorded.
func (s *Store) WriterBinary() (string, error) { return readWriterBinary(s.db) }

// SetWriterBinary records path as the database's last writer.
func (s *Store) SetWriterBinary(path string) error { return writeWriterBinary(s.db, path) }

func readWriterBinary(db *sql.DB) (string, error) {
	var path sql.NullString
	err := db.QueryRow(`SELECT CAST(version AS TEXT) FROM meta WHERE key = ?`, writerBinaryKey).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return path.String, err
}

func writeWriterBinary(db *sql.DB, path string) error {
	_, err := db.Exec(`INSERT INTO meta (key, version) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET version = excluded.version`, writerBinaryKey, path)
	return err
}

// recordWriterBinary stores the running binary's absolute path unless the row
// already names it, so a hook that opens the database pays a read, not a
// write, once the row is right. It is best effort: a read-only or locked
// database, or an executable that cannot be resolved, never fails the open.
func (s *Store) recordWriterBinary() {
	exe, err := executablePath()
	if err != nil {
		return
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	if current, err := readWriterBinary(s.db); err == nil && current == exe {
		return
	}
	_ = writeWriterBinary(s.db, exe)
}
