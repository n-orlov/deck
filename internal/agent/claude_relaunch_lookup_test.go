package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lookupTestCWD = "/tmp/relaunch-lookup/cwd"

const lookupTestID = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

// lookupTestDir returns the project directory Claude would use for the test cwd.
func lookupTestDir(home string) string {
	return filepath.Join(home, ".claude", "projects", strings.ReplaceAll(lookupTestCWD, string(filepath.Separator), "-"))
}

// R207 case 2 (conservative): a lookup failure that is not a confirmed
// not-exist leaves the transcript state unknown, so the relaunch resumes; only
// a confirmed absence relaunches fresh.
func TestClaudeRelaunchFreshDistinguishesAbsenceFromLookupFailure(t *testing.T) {
	const cwd = lookupTestCWD
	claude := NewClaude()
	in := func(home string) TranscriptInput {
		return TranscriptInput{Home: home, CWD: cwd, ConversationID: lookupTestID}
	}

	t.Run("confirmed absent: projects dir missing", func(t *testing.T) {
		if !claude.RelaunchFresh(in(t.TempDir())) {
			t.Error("RelaunchFresh = false, want true when nothing exists")
		}
	})
	t.Run("confirmed absent: project dir exists, file missing", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(lookupTestDir(home), 0o755); err != nil {
			t.Fatal(err)
		}
		if !claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = false, want true when the project dir is empty")
		}
	})
	t.Run("a directory in the transcript's place is no transcript", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(lookupTestDir(home), lookupTestID+".jsonl"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = false, want true for a directory at the transcript path")
		}
	})
	t.Run("lookup failure: a path component is a regular file (ENOTDIR)", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".claude", "projects"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = true, want false: ENOTDIR is not a confirmed not-exist")
		}
	})
	t.Run("lookup failure: path too long (ENAMETOOLONG)", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		long := TranscriptInput{Home: home, CWD: "/" + strings.Repeat("a", 300), ConversationID: lookupTestID}
		if claude.RelaunchFresh(long) {
			t.Error("RelaunchFresh = true, want false: ENAMETOOLONG is not a confirmed not-exist")
		}
	})
	t.Run("lookup failure: project dir is not searchable (EACCES)", func(t *testing.T) {
		home := t.TempDir()
		dir := lookupTestDir(home)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, lookupTestID+".jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if _, err := os.Lstat(filepath.Join(dir, lookupTestID+".jsonl")); err == nil {
			t.Skip("permission bits are not enforced for this user")
		}
		if claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = true, want false: an existing transcript behind an unreadable dir stays on --resume")
		}
	})
	t.Run("existing entry with unreadable metadata: dangling symlink", func(t *testing.T) {
		home := t.TempDir()
		dir := lookupTestDir(home)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(dir, lookupTestID+".jsonl")); err != nil {
			t.Fatal(err)
		}
		if claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = true, want false: the entry exists, so it must be resumed")
		}
		if p, ok := claude.TranscriptPaths(in(home)); ok {
			t.Errorf("TranscriptPaths = (%q, true) for a dangling symlink, want false", p)
		}
	})
	t.Run("existing transcript through a symlink resumes", func(t *testing.T) {
		home := t.TempDir()
		dir := lookupTestDir(home)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(home, "real.jsonl")
		if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, lookupTestID+".jsonl")); err != nil {
			t.Fatal(err)
		}
		if claude.RelaunchFresh(in(home)) {
			t.Error("RelaunchFresh = true, want false for an existing transcript")
		}
	})
}
