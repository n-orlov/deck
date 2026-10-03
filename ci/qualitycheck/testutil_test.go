package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempFile writes content to a new file under t.TempDir() and
// returns its path. Shared by every test in this package that needs a
// seeded ci/quality.json-shaped file on disk.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quality.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}
