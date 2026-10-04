package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestCheckResumeCWDMessages pins Resume's cwd rejection: an existing
// directory passes, while a missing path and a regular file are refused with
// a message naming the session and the cwd. The stat failure is rendered into
// the text and not wrapped, so errors.Is never sees fs.ErrNotExist.
func TestCheckResumeCWDMessages(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, statErr := os.Stat(missing)
	if statErr == nil {
		t.Fatalf("stat %q: want an error", missing)
	}

	if err := checkResumeCWD(store.Session{Name: "s", CWD: dir}); err != nil {
		t.Fatalf("existing directory: got %v, want nil", err)
	}

	err := checkResumeCWD(store.Session{Name: "s", CWD: missing})
	want := `resume session "s": cwd "` + missing + `" is missing or not a directory: ` + statErr.Error()
	if err == nil || err.Error() != want {
		t.Fatalf("missing cwd: got %v, want %q", err, want)
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Unwrap(err) != nil {
		t.Fatalf("missing cwd: error %q wraps a cause, want a plain message", err)
	}

	err = checkResumeCWD(store.Session{Name: "s", CWD: file})
	want = `resume session "s": cwd "` + file + `" is missing or not a directory`
	if err == nil || err.Error() != want {
		t.Fatalf("regular file: got %v, want %q", err, want)
	}
}
