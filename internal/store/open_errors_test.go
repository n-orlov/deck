package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPathRejectsEmptyPath(t *testing.T) {
	if _, err := OpenPath(t.TempDir(), ""); err == nil || err.Error() != "store database path is required" {
		t.Fatalf("OpenPath with no path error = %v", err)
	}
}

func TestOpenPathDerivesHomeFromPathAndExposesIt(t *testing.T) {
	home := filepath.Join(t.TempDir(), "derived")
	path := filepath.Join(home, "state.db")
	s, err := OpenPath("", path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Path() != path {
		t.Fatalf("Path() = %q; want %q", s.Path(), path)
	}
	assertMode(t, home, 0o700)
}

func TestOpenPathFailsWhenHomeCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenPath(filepath.Join(blocker, "deck"), filepath.Join(blocker, "deck", "state.db"))
	if err == nil || !strings.Contains(err.Error(), "create store directory") {
		t.Fatalf("error = %v; want create store directory failure", err)
	}
}

func TestOpenPathRefusesFileThatIsNotADatabase(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	garbage := []byte(strings.Repeat("this is not a sqlite database. ", 20))
	if err := os.WriteFile(path, garbage, 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenPath(home, path); err == nil {
		s.Close()
		t.Fatal("OpenPath on a non-database file succeeded; want an error")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(garbage) {
		t.Fatalf("the refused file was modified: %v", err)
	}
}
