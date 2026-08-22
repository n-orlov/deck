package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

	t.Run("mode change", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "permissioned.txt")
		if err := os.WriteFile(target, []byte("same bytes, different mode"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		// Change mode only, leaving content and size untouched. Restore the
		// mtime explicitly afterwards in case the platform bumps it on
		// chmod, so this subtest isolates a mode change from the mtime-touch
		// mode above: a naive implementation that never compares Mode must
		// still fail here.
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat before chmod: %v", err)
		}
		originalModTime := info.ModTime()
		if err := os.Chmod(target, 0o444); err != nil {
			t.Fatalf("chmod file: %v", err)
		}
		if err := os.Chtimes(target, originalModTime, originalModTime); err != nil {
			t.Fatalf("restore mtime after chmod: %v", err)
		}
		// Restore write permission so t.TempDir()'s own cleanup can remove
		// the file afterwards.
		t.Cleanup(func() { _ = os.Chmod(target, 0o644) })

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect a mode change")
		}
		if !strings.Contains(err.Error(), "permissioned.txt") {
			t.Fatalf("compareFingerprints error %q does not name the mode-changed path", err.Error())
		}
	})

	t.Run("symlink target swap", func(t *testing.T) {
		root := t.TempDir()
		originalTarget := filepath.Join(root, "original-target.txt")
		swappedTarget := filepath.Join(root, "swapped-target.txt")
		// Identical content in both targets, so only the symlink's own
		// target string differs -- a naive implementation that follows the
		// symlink and fingerprints the resolved file's bytes would see no
		// difference at all here.
		sameContent := []byte("identical content in both targets")
		if err := os.WriteFile(originalTarget, sameContent, 0o644); err != nil {
			t.Fatalf("seed original target: %v", err)
		}
		if err := os.WriteFile(swappedTarget, sameContent, 0o644); err != nil {
			t.Fatalf("seed swapped target: %v", err)
		}
		linkPath := filepath.Join(root, "link")
		if err := os.Symlink(originalTarget, linkPath); err != nil {
			t.Fatalf("seed symlink: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		// Record the symlink's own lstat mtime before repointing it, so it
		// can be restored below: recreating a symlink otherwise bumps its
		// own mtime, which would let the mtime-touch comparison catch this
		// mutation for the wrong reason and leave the "do not follow
		// symlinks" decision unpinned.
		linkInfoBefore, err := os.Lstat(linkPath)
		if err != nil {
			t.Fatalf("lstat symlink before repointing: %v", err)
		}
		originalLinkModTime := linkInfoBefore.ModTime()

		if err := os.Remove(linkPath); err != nil {
			t.Fatalf("remove symlink before repointing: %v", err)
		}
		if err := os.Symlink(swappedTarget, linkPath); err != nil {
			t.Fatalf("repoint symlink: %v", err)
		}
		// os.Chtimes follows symlinks, so it cannot restore the symlink's
		// own mtime; use AT_SYMLINK_NOFOLLOW directly.
		ts := unix.NsecToTimespec(originalLinkModTime.UnixNano())
		if err := unix.UtimesNanoAt(unix.AT_FDCWD, linkPath, []unix.Timespec{ts, ts}, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			t.Fatalf("restore symlink's own mtime after repointing: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect a symlink target swap")
		}
		if !strings.Contains(err.Error(), "link") {
			t.Fatalf("compareFingerprints error %q does not name the repointed link", err.Error())
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

	t.Run("root directory mode change", func(t *testing.T) {
		// fingerprintDirectory records the fingerprinted root's own mode
		// under rootFingerprintKey (".") -- a chmod of the directory being
		// fingerprinted, not merely of something inside it, must still be
		// caught.
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("untouched"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		before, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint before mutation: %v", err)
		}

		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat root before chmod: %v", err)
		}
		originalModTime := info.ModTime()
		if err := os.Chmod(root, 0o700); err != nil {
			t.Fatalf("chmod root: %v", err)
		}
		if err := os.Chtimes(root, originalModTime, originalModTime); err != nil {
			t.Fatalf("restore root mtime after chmod: %v", err)
		}

		after, err := fingerprintDirectory(root)
		if err != nil {
			t.Fatalf("fingerprint after mutation: %v", err)
		}
		err = compareFingerprints(before, after)
		if err == nil {
			t.Fatalf("compareFingerprints did not detect the fingerprinted root's own mode change")
		}
		if !strings.Contains(err.Error(), rootFingerprintKey) {
			t.Fatalf("compareFingerprints error %q does not name the root's reserved key %q", err.Error(), rootFingerprintKey)
		}
	})
}
