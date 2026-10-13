package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// fakeClock is a settable clock for the 24-hour window.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newChecker(url, cache string, clock *fakeClock, enabled bool) *Checker {
	return &Checker{
		Enabled:   func() bool { return enabled },
		CachePath: cache,
		Options:   Options{BaseURL: url, Current: "v1.0.0"},
		Now:       clock.now,
	}
}

// TestCheckThrottle: the fake release server counts requests across four rows.
func TestCheckThrottle(t *testing.T) {
	t.Run("a second check within 24h makes 0 requests, one after 24h makes 1", func(t *testing.T) {
		srv, hits := countingServer(t)
		clock := &fakeClock{t: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
		c := newChecker(srv.URL, filepath.Join(t.TempDir(), "update-check.json"), clock, true)
		first := c.Check(context.Background())
		if !first.Requested || !first.Newer || first.Latest != tagV || hits.Load() == 0 {
			t.Fatalf("first check = %+v hits=%d; want a request finding %s", first, hits.Load(), tagV)
		}
		afterFirst := hits.Load()
		clock.advance(23*time.Hour + 59*time.Minute)
		second := c.Check(context.Background())
		if hits.Load() != afterFirst || second.Requested {
			t.Fatalf("a check inside 24h made %d request(s), want 0", hits.Load()-afterFirst)
		}
		if !second.Newer || second.Latest != tagV {
			t.Fatalf("a cached answer = %+v; want the indicator kept", second)
		}
		clock.advance(2 * time.Minute)
		third := c.Check(context.Background())
		if !third.Requested || hits.Load() == afterFirst {
			t.Fatalf("a check after 24h made no request (%+v)", third)
		}
		// One check resolves the latest tag with a single GET on /releases/latest.
		if got := hits.Load() - afterFirst; got != 1 {
			t.Fatalf("a check after 24h made %d requests, want 1", got)
		}
	})
	t.Run("update_check=false makes 0 requests", func(t *testing.T) {
		srv, hits := countingServer(t)
		clock := &fakeClock{t: time.Now()}
		cache := filepath.Join(t.TempDir(), "update-check.json")
		st := newChecker(srv.URL, cache, clock, false).Check(context.Background())
		if hits.Load() != 0 || st != (Status{}) {
			t.Fatalf("a disabled check made %d requests, status %+v", hits.Load(), st)
		}
		if _, err := os.Stat(cache); err == nil {
			t.Fatal("a disabled check wrote the cache")
		}
	})
	t.Run("a failed or offline check shows no indicator, no error, no retry inside the window", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()
		clock := &fakeClock{t: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
		c := newChecker(srv.URL, filepath.Join(t.TempDir(), "update-check.json"), clock, true)
		st := c.Check(context.Background())
		if st.Newer || st.Latest != "" || hits.Load() != 1 {
			t.Fatalf("failed check = %+v hits=%d; want no indicator after one request", st, hits.Load())
		}
		for range 3 {
			clock.advance(time.Hour)
			if st := c.Check(context.Background()); st.Newer || st.Requested {
				t.Fatalf("retry inside the window: %+v", st)
			}
		}
		if hits.Load() != 1 {
			t.Fatalf("a failed check was retried inside the window: %d requests", hits.Load())
		}
	})
	t.Run("offline", func(t *testing.T) {
		srv, _ := countingServer(t)
		url := srv.URL
		srv.Close()
		clock := &fakeClock{t: time.Now()}
		c := newChecker(url, filepath.Join(t.TempDir(), "update-check.json"), clock, true)
		if st := c.Check(context.Background()); st.Newer || st.Latest != "" {
			t.Fatalf("offline check = %+v; want no indicator", st)
		}
	})
}

// TestCheckRunsOffUIGoroutine: Start returns while the server is still
// holding the request, and the answer arrives later on the channel.
func TestCheckRunsOffUIGoroutine(t *testing.T) {
	release := make(chan struct{})
	inner, _ := countingServer(t)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer slow.Close()
	clock := &fakeClock{t: time.Now()}
	c := newChecker(slow.URL, filepath.Join(t.TempDir(), "update-check.json"), clock, true)
	returned := make(chan (<-chan Status), 1)
	go func() { returned <- c.Start(context.Background()) }()
	var ch <-chan Status
	select {
	case ch = <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Start blocked on the network")
	}
	select {
	case st := <-ch:
		t.Fatalf("the answer arrived before the server answered: %+v", st)
	default:
	}
	close(release)
	select {
	case st := <-ch:
		if !st.Newer {
			t.Fatalf("status = %+v; want Newer", st)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no status after the server answered")
	}
}

// TestCheckCacheSharedAcrossProfiles: two profiles resolve one cache file
// that is not either profile's state.db; one profile's check is read by the
// other, which then makes no request, and both sides write the same file.
func TestCheckCacheSharedAcrossProfiles(t *testing.T) {
	root := t.TempDir()
	env := func(k string) string {
		if k == "DECK_HOME" {
			return root
		}
		return ""
	}
	home := func() (string, error) { return root, nil }
	a, err := config.LoadFromProfile(env, home, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := config.LoadFromProfile(env, home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if a.Paths.UpdateCache == "" || a.Paths.UpdateCache != b.Paths.UpdateCache {
		t.Fatalf("profiles resolve different caches: %q vs %q", a.Paths.UpdateCache, b.Paths.UpdateCache)
	}
	if a.Paths.UpdateCache == a.Paths.StateDB || a.Paths.UpdateCache == b.Paths.StateDB {
		t.Fatal("the cache is a state.db")
	}
	srv, hits := countingServer(t)
	clock := &fakeClock{t: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	ca := newChecker(srv.URL, a.Paths.UpdateCache, clock, true)
	cb := newChecker(srv.URL, b.Paths.UpdateCache, clock, true)
	if st := ca.Check(context.Background()); !st.Requested || !st.Newer {
		t.Fatalf("profile A check = %+v", st)
	}
	n := hits.Load()
	if st := cb.Check(context.Background()); st.Requested || !st.Newer || hits.Load() != n {
		t.Fatalf("profile B did not read A's cache: %+v, %d new requests", st, hits.Load()-n)
	}
	clock.advance(25 * time.Hour)
	if st := cb.Check(context.Background()); !st.Requested {
		t.Fatalf("profile B after 24h = %+v; want a request", st)
	}
	n = hits.Load()
	if st := ca.Check(context.Background()); st.Requested || hits.Load() != n {
		t.Fatalf("profile A did not read B's write: %+v", st)
	}
	if entries, _ := os.ReadDir(filepath.Dir(a.Paths.UpdateCache)); len(entries) == 0 {
		t.Fatal("no cache file on disk")
	}
	if _, err := os.Stat(a.Paths.UpdateCache); err != nil {
		t.Fatalf("shared cache missing: %v", err)
	}
}

// TestCheckCacheWriteFailureIsSilent: a cache that cannot be written (its
// parent is a regular file) neither fails the check nor leaves a stray file.
func TestCheckCacheWriteFailureIsSilent(t *testing.T) {
	srv, _ := countingServer(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newChecker(srv.URL, filepath.Join(blocker, "update-check.json"), &fakeClock{t: time.Now()}, true)
	if st := c.Check(context.Background()); !st.Newer {
		t.Fatalf("status = %+v; want the check to succeed without a cache", st)
	}
	if err := writeAtomic(filepath.Join(dir, "gone", "x.json"), []byte("{}")); err != nil {
		t.Fatalf("writeAtomic creates its directory: %v", err)
	}
	if err := writeAtomic(dir, []byte("{}")); err == nil {
		t.Fatal("writeAtomic over a directory succeeded")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".update-check-*")); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}
