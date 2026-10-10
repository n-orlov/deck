package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The "fake claude agent's size log is captured" step must snapshot the
// fixture's own initial entry, never race the fixture's startup inside its
// tmux pane (push run of e9a3565, preview.feature:28: the log did not exist
// yet when the step read it once).

func TestFakeClaudeSizeSnapshotWaitsForTheInitialEntryWrittenAfterTheStepStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "fake-claude-sizes.log")
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.MkdirAll(filepath.Dir(path), 0o750)
		_ = os.WriteFile(path, []byte("80x24\n"), 0o600)
	}()
	got, err := waitForFakeClaudeInitialSizeEntry(path, 5*time.Second)
	if err != nil {
		t.Fatalf("wait for initial entry: %v", err)
	}
	if got != "80x24\n" {
		t.Fatalf("snapshot = %q, want the fixture's initial entry %q", got, "80x24\n")
	}
}

func TestFakeClaudeSizeSnapshotWaitsForAPartialLineToComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-claude-sizes.log")
	if err := os.WriteFile(path, []byte("80x2"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(path, []byte("80x24\n"), 0o600)
	}()
	got, err := waitForFakeClaudeInitialSizeEntry(path, 5*time.Second)
	if err != nil {
		t.Fatalf("wait for initial entry: %v", err)
	}
	if got != "80x24\n" {
		t.Fatalf("snapshot = %q, want the completed line %q", got, "80x24\n")
	}
}

func TestFakeClaudeSizeSnapshotFailsWhenTheFixtureNeverWritesItsEntry(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "fake-claude-sizes.log")
	if _, err := waitForFakeClaudeInitialSizeEntry(missing, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no initial entry") {
		t.Fatalf("missing log: err = %v, want a no-initial-entry error, never an empty snapshot", err)
	}
	partial := filepath.Join(dir, "partial.log")
	if err := os.WriteFile(partial, []byte("80x2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForFakeClaudeInitialSizeEntry(partial, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no complete initial entry") {
		t.Fatalf("partial log: err = %v, want a no-complete-initial-entry error", err)
	}
	if _, err := waitForFakeClaudeInitialSizeEntry(dir, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "read fake claude agent size log") {
		t.Fatalf("unreadable path: err = %v, want a read error", err)
	}
}
