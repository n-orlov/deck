package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAtomicWriteFailureLeavesTargetAndNoTempFile pins the contract in
// atomicWrite's comment: a step that fails before or at the rename returns
// a stated error naming that step, never touches the real path's existing
// content, and leaves no half-written temp file behind.
func TestAtomicWriteFailureLeavesTargetAndNoTempFile(t *testing.T) {
	t.Run("parent is a regular file", func(t *testing.T) {
		root := t.TempDir()
		blocker := filepath.Join(root, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := atomicWrite(filepath.Join(blocker, "config.toml"), []byte("data"))
		if err == nil || !strings.Contains(err.Error(), "create "+blocker) {
			t.Fatalf("error = %v, want one naming the directory it could not create", err)
		}
	})

	t.Run("target is a directory", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "config.toml")
		if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
			t.Fatal(err)
		}
		err := atomicWrite(target, []byte("data"))
		if err == nil || !strings.Contains(err.Error(), "rename ") {
			t.Fatalf("error = %v, want a rename failure", err)
		}
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 1 || entries[0].Name() != "config.toml" {
			t.Fatalf("directory holds %v after the failed write, want only config.toml (no temp file left)", entries)
		}
	})
}

// TestWriteTempFileNamesTheFailingStep covers writeTempFile's error exits:
// each wraps the temp file's own path and the underlying cause.
func TestWriteTempFileNamesTheFailingStep(t *testing.T) {
	t.Run("write to a closed file", func(t *testing.T) {
		tmp, err := os.CreateTemp(t.TempDir(), "w.*.tmp")
		if err != nil {
			t.Fatal(err)
		}
		if err := tmp.Close(); err != nil {
			t.Fatal(err)
		}
		err = writeTempFile(tmp, []byte("data"))
		if err == nil || !strings.Contains(err.Error(), "write "+tmp.Name()) {
			t.Fatalf("error = %v, want one naming the write of %s", err, tmp.Name())
		}
	})

	t.Run("temp file vanished before chmod", func(t *testing.T) {
		tmp, err := os.CreateTemp(t.TempDir(), "w.*.tmp")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(tmp.Name()); err != nil {
			t.Fatal(err)
		}
		err = writeTempFile(tmp, []byte("data"))
		if err == nil || !strings.Contains(err.Error(), "chmod "+tmp.Name()) {
			t.Fatalf("error = %v, want one naming the chmod of %s", err, tmp.Name())
		}
	})
}
