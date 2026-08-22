package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFingerprintDetectsMutations proves task 003's requirement: the
// fingerprint instrument used by every destructive-path proof in this phase
// (tasks 036/037) is not a rubber stamp. Each subtest seeds a directory,
// takes a fingerprint, applies exactly one kind of mutation, and asserts
// compareFingerprints reports a non-nil error naming the differing path. If
// any of the four mutation modes went undetected, this test fails.
func TestFingerprintDetectsMutations(t *testing.T) {
	t.Run("content change", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "unchanged-name.txt")
		if err := os.WriteFile(target, []byte("original content"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		// Mutate content only. Preserve mtime explicitly so this subtest
		// isolates content change from the mtime-touch mode below: a naive
		// implementation that only compares mtime, not bytes, must still
		// fail here.
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat before rewrite: %v", err)
		}
		originalModTime := info.ModTime()
		if err := os.WriteFile(target, []byte("mutated content!"), 0o644); err != nil {
			t.Fatalf("rewrite file: %v", err)
		}
		if err := os.Chtimes(target, originalModTime, originalModTime); err != nil {
			t.Fatalf("restore mtime after rewrite: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect a content change")
		}
		if !strings.Contains(err.Error(), "unchanged-name.txt") {
			t.Fatalf("compareFingerprints error %q does not name the mutated path", err.Error())
		}
	})

	t.Run("mtime touch", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "touched.txt")
		if err := os.WriteFile(target, []byte("same bytes throughout"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		// Touch the mtime forward without touching content, mode or size:
		// a naive implementation that only compares content/size must
		// still fail here.
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat before touch: %v", err)
		}
		newModTime := info.ModTime().Add(1 * time.Hour)
		if err := os.Chtimes(target, newModTime, newModTime); err != nil {
			t.Fatalf("touch mtime: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect an mtime touch")
		}
		if !strings.Contains(err.Error(), "touched.txt") {
			t.Fatalf("compareFingerprints error %q does not name the touched path", err.Error())
		}
	})

	t.Run("added file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("stays put"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		addedPath := filepath.Join(root, "intruder.txt")
		if err := os.WriteFile(addedPath, []byte("should not be here"), 0o644); err != nil {
			t.Fatalf("add file: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect an added file")
		}
		if !strings.Contains(err.Error(), "intruder.txt") {
			t.Fatalf("compareFingerprints error %q does not name the added path", err.Error())
		}
	})

	t.Run("removed file", func(t *testing.T) {
		root := t.TempDir()
		removedPath := filepath.Join(root, "victim.txt")
		if err := os.WriteFile(removedPath, []byte("will be removed"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "survivor.txt"), []byte("stays put"), 0o644); err != nil {
			t.Fatalf("seed second file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		if err := os.Remove(removedPath); err != nil {
			t.Fatalf("remove file: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect a removed file")
		}
		if !strings.Contains(err.Error(), "victim.txt") {
			t.Fatalf("compareFingerprints error %q does not name the removed path", err.Error())
		}
	})
}
