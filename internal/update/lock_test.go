package update

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/testrelbin"
)

// archiveHits counts requests to the release archive of rel served by srv.
func archiveHits(t *testing.T, rel *release) (baseURL string, hits *atomic.Int64) {
	t.Helper()
	srv := rel.serve(t)
	orig := srv.Config.Handler
	hits = new(atomic.Int64)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == rel.asset {
			hits.Add(1)
		}
		orig.ServeHTTP(w, r)
	})
	return srv.URL, hits
}

// installAt is an install whose executable is a real Go binary stamped tag.
func installAt(t *testing.T, tag string) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "deck")
	if err := os.WriteFile(exe, testrelbin.Bytes(t, tag), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

// lockIsFree reports whether a LOCK_EX|LOCK_NB flock on path succeeds.
func lockIsFree(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open the lock file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return true
}

func lockPathIn(t *testing.T) string {
	t.Helper()
	return InstallLockPath(filepath.Join(t.TempDir(), "cache", "update-check.json"))
}

// TestOnlyIfNewerRechecksTheBinaryOnDisk: the running build's Current is
// stale, but another deck already installed the latest tag over the target;
// under OnlyIfNewer nothing is downloaded.
func TestOnlyIfNewerRechecksTheBinaryOnDisk(t *testing.T) {
	base, hits := archiveHits(t, newRelease(t))
	dir, exe := installAt(t, tagV)
	before := fileSum(t, exe)
	res, err := Run(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v1.0.0", OnlyIfNewer: true, InstallLock: lockPathIn(t)})
	if err != nil || !res.Unchanged || res.Tag != tagV {
		t.Fatalf("Run = %+v, %v; want Unchanged at %s", res, err, tagV)
	}
	if hits.Load() != 0 {
		t.Errorf("%d archive requests, want none", hits.Load())
	}
	if fileSum(t, exe) != before || len(listing(t, dir)) != 1 {
		t.Error("an install already at the latest tag was touched")
	}
}

// TestExplicitUpdateInstallsEvenWhenTargetIsAtTag: without OnlyIfNewer the
// on-disk version does not stop the install.
func TestExplicitUpdateInstallsEvenWhenTargetIsAtTag(t *testing.T) {
	base, hits := archiveHits(t, newRelease(t))
	_, exe := installAt(t, tagV)
	res, err := Run(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v1.0.0", InstallLock: lockPathIn(t)})
	if err != nil || res.Unchanged || res.Tag != tagV {
		t.Fatalf("Run = %+v, %v; want an install of %s", res, err, tagV)
	}
	if hits.Load() == 0 {
		t.Error("no archive was requested; the install did not happen")
	}
}

// TestInstallLockUncreatableProceedsUnlocked: a lock file that cannot be
// created is no reason to refuse the update.
func TestInstallLockUncreatableProceedsUnlocked(t *testing.T) {
	srv := newRelease(t).serve(t)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, exe := installOld(t)
	res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, InstallLock: filepath.Join(file, "sub", InstallLockName)})
	if err != nil || res.Tag != tagV {
		t.Fatalf("Run = %+v, %v; want an install", res, err)
	}
	if fileSum(t, exe) != sum([]byte(newBody(t))) {
		t.Error("the binary was not replaced")
	}
}

func TestInstallLockReleasedAfterSuccess(t *testing.T) {
	srv := newRelease(t).serve(t)
	_, exe := installOld(t)
	lock := lockPathIn(t)
	if _, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, InstallLock: lock}); err != nil {
		t.Fatal(err)
	}
	if !lockIsFree(t, lock) {
		t.Error("the install lock is still held after a successful Run")
	}
}

func TestInstallLockReleasedAfterFailure(t *testing.T) {
	rel := newRelease(t)
	rel.missing["/releases/download/"+tagV+"/"+rel.asset] = true
	srv := rel.serve(t)
	_, exe := installOld(t)
	lock := lockPathIn(t)
	if _, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, InstallLock: lock}); err == nil {
		t.Fatal("Run succeeded with the archive missing")
	}
	if !lockIsFree(t, lock) {
		t.Error("the install lock is still held after a failed Run")
	}
}

// TestInstallLockWaiterCancelledReturnsContextError: an install waiting on
// another holder gives up when its context ends, with the context's error,
// the binary unchanged and no lock held.
func TestInstallLockWaiterCancelledReturnsContextError(t *testing.T) {
	base, hits := archiveHits(t, newRelease(t))
	dir, exe := installOld(t)
	lock := lockPathIn(t)
	unlock, ok := flockPath(context.Background(), lock)
	if !ok {
		t.Fatal("could not take the lock")
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := Run(ctx, Options{BaseURL: base, Exe: exe, InstallLock: lock})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v; want one satisfying errors.Is(context.Canceled)", err)
	}
	if hits.Load() != 0 || fileSum(t, exe) != sum([]byte(oldBody)) || len(listing(t, dir)) != 1 {
		t.Error("a cancelled waiter touched the binary or downloaded")
	}
	unlock()
	if !lockIsFree(t, lock) {
		t.Error("the cancelled waiter left the lock held")
	}
}

// TestConcurrentOnlyIfNewerInstallsDownloadOnce: two OnlyIfNewer installs
// racing on one target, one install lock and one release server whose archive
// download is slow enough that the calls overlap. The loser waits on the lock,
// re-reads the binary on disk and finds the tag already installed, so the
// archive is downloaded and installed once.
func TestConcurrentOnlyIfNewerInstallsDownloadOnce(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	orig := srv.Config.Handler
	var hits atomic.Int64
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == rel.asset {
			hits.Add(1)
			time.Sleep(400 * time.Millisecond) // keep the other call in flight meanwhile
		}
		orig.ServeHTTP(w, r)
	})
	_, exe := installOld(t)
	lock := lockPathIn(t)

	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, Current: "v1.0.0", OnlyIfNewer: true, InstallLock: lock})
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Run #%d: %v", i, err)
		}
	}
	unchanged := 0
	for _, r := range results {
		if r.Unchanged {
			unchanged++
		}
		if r.Tag != tagV {
			t.Errorf("Result.Tag = %q, want %q", r.Tag, tagV)
		}
	}
	if unchanged != 1 {
		t.Errorf("%d of 2 results Unchanged, want exactly 1 (results: %+v)", unchanged, results)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d archive downloads, want exactly 1", n)
	}
	if got := (Options{}).installedVersion(exe); got != tagV {
		t.Errorf("target build info reads %q, want %q", got, tagV)
	}
}
