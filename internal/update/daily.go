package update

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// CheckInterval is how long a check's answer, success or failure, is reused:
// the daily check asks the release server at most once per interval.
const CheckInterval = 24 * time.Hour

// cacheRecord is the shared update-check cache file's whole content. It holds
// the time of the last attempt and the latest release tag it saw; nothing
// else, and never a credential.
type cacheRecord struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest,omitempty"`
}

// Status is the outcome of one daily check.
type Status struct {
	// Latest is the newest stable release tag known, or "" when the check is
	// disabled, failed, or has not found one.
	Latest string
	// Newer is set when Latest is a newer stable release than the running
	// build: it is the only field a UI draws an indicator from. A failed or
	// offline check, a disabled check and a dev build never set it.
	Newer bool
	// Requested is set when this call made the network request (false when
	// the answer came from the cache or the check is off).
	Requested bool
}

// Checker is the throttled daily update check. Its cache file is shared by
// every profile (config.Paths.UpdateCache) and sits outside every state.db.
type Checker struct {
	// Enabled reports whether the check may run (update_check or auto_update
	// on). Read at each call, so a live settings change applies at once.
	Enabled func() bool
	// CachePath is the shared cache file. Empty disables caching: every call
	// then asks the server, so callers always set it.
	CachePath string
	// Options carries BaseURL, Current and Client for the request.
	Options Options
	// Now is the clock; time.Now when nil.
	Now func() time.Time

	mu sync.Mutex // serialises the read-decide-write of one process's checks
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Check answers from the cache when its last attempt is under CheckInterval
// old; otherwise it makes one request and records the attempt. A failure is
// recorded like a success (so an offline machine does not retry inside the
// window), is never returned as an error and shows no indicator.
func (c *Checker) Check(ctx context.Context) Status {
	if c.Enabled != nil && !c.Enabled() {
		return Status{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// The advisory file lock makes read -> decide -> request -> record one
	// operation across every process and profile sharing the cache; the
	// mutex above only orders this Checker's own calls.
	unlock, ok := c.lock(ctx)
	if !ok {
		return Status{}
	}
	defer unlock()
	now := c.now()
	if rec, ok := c.read(); ok && now.Sub(rec.CheckedAt) < CheckInterval && !rec.CheckedAt.After(now) {
		return c.status(rec.Latest, false)
	}
	res, err := Check(ctx, c.Options)
	rec := cacheRecord{CheckedAt: now}
	if err == nil {
		rec.Latest = res.Latest
	}
	c.write(rec)
	return c.status(rec.Latest, true)
}

// Start runs Check on its own goroutine and delivers the Status on the
// returned channel (buffered: the sender never blocks on a reader that has
// gone away). It returns at once, so the UI goroutine never waits on the
// network or the cache file.
func (c *Checker) Start(ctx context.Context) <-chan Status {
	out := make(chan Status, 1)
	go func() { out <- c.Check(ctx) }()
	return out
}

// lockPoll is how often a check waiting on another process's check retries
// the cache lock.
const lockPoll = 20 * time.Millisecond

// lock takes the exclusive advisory lock on the cache's sibling lock file and
// returns its release. A check that cannot create the lock file (an
// unwritable cache directory: there is no shared cache to coordinate on)
// proceeds unlocked, like every other cache failure; ok is false only when
// ctx ends while waiting, in which case no request is made and nothing is
// recorded.
func (c *Checker) lock(ctx context.Context) (unlock func(), ok bool) {
	if c.CachePath == "" {
		return func() {}, true
	}
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o750); err != nil {
		return func() {}, true
	}
	f, err := os.OpenFile(c.CachePath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
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

func (c *Checker) status(latest string, requested bool) Status {
	return Status{Latest: latest, Newer: IsNewer(latest, c.Options.Current), Requested: requested}
}

func (c *Checker) read() (cacheRecord, bool) {
	if c.CachePath == "" {
		return cacheRecord{}, false
	}
	b, err := os.ReadFile(c.CachePath)
	if err != nil {
		return cacheRecord{}, false
	}
	var rec cacheRecord
	if json.Unmarshal(b, &rec) != nil || rec.CheckedAt.IsZero() {
		return cacheRecord{}, false
	}
	return rec, true
}

// write stores rec; a failure is dropped: the cache is an optimisation and
// the check must not fail on it.
func (c *Checker) write(rec cacheRecord) {
	if c.CachePath == "" {
		return
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_ = writeAtomic(c.CachePath, append(b, '\n'))
}

// writeAtomic replaces path through a temp file in the same directory and a
// rename, so two profiles never read a torn file, and leaves no temp file
// behind on any failure.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".update-check-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Close(), os.Rename(tmp.Name(), path)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
