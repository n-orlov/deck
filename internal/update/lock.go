package update

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockPoll is how often a process waiting on another's advisory lock retries.
const lockPoll = 20 * time.Millisecond

// flockPath takes an exclusive advisory flock (no PID file) on the file at
// path, creating it and its directory when needed, and returns its release.
// A lock file that cannot be created or locked means there is nothing to
// coordinate on, so the caller proceeds unlocked (ok true, a no-op unlock);
// ok is false only when ctx ends while waiting, and then nothing is held.
func flockPath(ctx context.Context, path string) (unlock func(), ok bool) {
	f := openLockFile(path)
	if f == nil {
		return func() {}, true
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, true
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			_ = f.Close()
			return func() {}, true
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, false
		case <-time.After(lockPoll):
		}
	}
}

// openLockFile opens the lock file at path, or returns nil when path is empty
// or its directory or the file cannot be created.
func openLockFile(path string) *os.File {
	if path == "" || os.MkdirAll(filepath.Dir(path), 0o750) != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil
	}
	return f
}

// InstallLockName is the install lock's file name, beside the update cache.
const InstallLockName = "update-install.lock"

// InstallLockPath is the machine-wide install lock for the update cache at
// cachePath: a file in the cache's directory, distinct from the cache's own
// check lock (cachePath + ".lock"). An empty cachePath gives no lock.
func InstallLockPath(cachePath string) string {
	if cachePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cachePath), InstallLockName)
}
