package update

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// slowServer answers every request after a short pause, with status code, so
// concurrent checks overlap inside the request.
func slowServer(t *testing.T, code int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(150 * time.Millisecond)
		if code != http.StatusOK {
			http.Error(w, "boom", code)
			return
		}
		http.Redirect(w, r, "/n-orlov/deck/releases/tag/"+tagV, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// runConcurrent runs n separate Checkers (one per instance/profile, none
// sharing a mutex) on one cache at once and returns how many made a request.
func runConcurrent(t *testing.T, url, cache string, clock *fakeClock, n int) int {
	t.Helper()
	var wg sync.WaitGroup
	var requested atomic.Int32
	start := make(chan struct{})
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newChecker(url, cache, clock, true)
			<-start
			if c.Check(context.Background()).Requested {
				requested.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	return int(requested.Load())
}

// TestCheckConcurrentInstancesMakeOneRequest: instances and profiles that
// share the cache collectively make one request per window, for a success and
// a failure alike, and again for the next window.
func TestCheckConcurrentInstancesMakeOneRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{{"success", http.StatusOK}, {"server error", http.StatusInternalServerError}, {"not found", http.StatusNotFound}} {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := slowServer(t, tc.code)
			cache := filepath.Join(t.TempDir(), "nested", "update-check.json")
			clock := &fakeClock{t: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
			if got := runConcurrent(t, srv.URL, cache, clock, 8); got != 1 || hits.Load() != 1 {
				t.Fatalf("8 concurrent checks: %d requested, %d server hits; want 1 and 1", got, hits.Load())
			}
			clock.advance(23 * time.Hour)
			if got := runConcurrent(t, srv.URL, cache, clock, 6); got != 0 || hits.Load() != 1 {
				t.Fatalf("inside the window: %d requested, %d hits; want 0 and 1", got, hits.Load())
			}
			clock.advance(2 * time.Hour)
			if got := runConcurrent(t, srv.URL, cache, clock, 6); got != 1 || hits.Load() != 2 {
				t.Fatalf("next window: %d requested, %d hits; want 1 and 2", got, hits.Load())
			}
		})
	}
}

// TestCheckConcurrentProcessesMakeOneRequest: separate OS processes, each
// with its own Checker, make one request between them.
func TestCheckConcurrentProcessesMakeOneRequest(t *testing.T) {
	srv, hits := slowServer(t, http.StatusOK)
	cache := filepath.Join(t.TempDir(), "update-check.json")
	const procs = 5
	var wg sync.WaitGroup
	var requested atomic.Int32
	for range procs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCheckProcessHelper$") //nolint:gosec // G702: re-executes this test binary
			cmd.Env = append(os.Environ(), "DECK_UPDATE_HELPER_URL="+srv.URL, "DECK_UPDATE_HELPER_CACHE="+cache)
			out, err := cmd.Output()
			if err != nil {
				t.Errorf("helper: %v", err)
				return
			}
			if bytes.Contains(out, []byte("REQUESTED")) {
				requested.Add(1)
			}
		}()
	}
	wg.Wait()
	if requested.Load() != 1 || hits.Load() != 1 {
		t.Fatalf("%d processes: %d requested, %d server hits; want 1 and 1", procs, requested.Load(), hits.Load())
	}
}

// TestCheckProcessHelper is the child side of the multi-process test.
func TestCheckProcessHelper(t *testing.T) {
	url := os.Getenv("DECK_UPDATE_HELPER_URL")
	if url == "" {
		t.Skip("child-process helper")
	}
	c := &Checker{CachePath: os.Getenv("DECK_UPDATE_HELPER_CACHE"), Options: Options{BaseURL: url, Current: "v1.0.0"}}
	if c.Check(context.Background()).Requested {
		_, _ = os.Stdout.WriteString("REQUESTED\n")
	}
}

// TestCheckWaitingForTheLockHonoursContext: a check that cannot get the cache
// lock before its context ends makes no request and records nothing.
func TestCheckWaitingForTheLockHonoursContext(t *testing.T) {
	srv, hits := countingServer(t)
	cache := filepath.Join(t.TempDir(), "update-check.json")
	holder := &Checker{CachePath: cache}
	unlock, ok := holder.lock(context.Background())
	if !ok {
		t.Fatal("could not take the lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	st := newChecker(srv.URL, cache, &fakeClock{t: time.Now()}, true).Check(ctx)
	if st != (Status{}) || hits.Load() != 0 {
		t.Fatalf("status %+v, %d hits; want none while the lock is held", st, hits.Load())
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("a check that never got the lock recorded an attempt")
	}
	unlock()
	if st := newChecker(srv.URL, cache, &fakeClock{t: time.Now()}, true).Check(context.Background()); !st.Requested {
		t.Fatalf("after release: %+v; want a request", st)
	}
}
