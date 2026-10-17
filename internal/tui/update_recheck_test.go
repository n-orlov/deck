package tui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	upd "github.com/n-orlov/deck/internal/update"
)

// #119 / SPEC §6.5: the daily check runs again from a slow tick while deck
// stays open, so a long-lived deck sees a release published after it started.
// The check stays throttled by the shared cache: one request per interval
// across every instance, and an instance that did not ask reads the answer
// another one recorded.

// recheckServer is a loopback fake release server whose latest tag can change
// and which counts the checks it answers.
type recheckServer struct {
	*httptest.Server
	mu     sync.Mutex
	latest string
	checks atomic.Int32
}

func newRecheckServer(t *testing.T, latest string) *recheckServer {
	t.Helper()
	s := &recheckServer{latest: latest}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		s.checks.Add(1)
		s.mu.Lock()
		tag := s.latest
		s.mu.Unlock()
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *recheckServer) publish(tag string) {
	s.mu.Lock()
	s.latest = tag
	s.mu.Unlock()
}

// recheckClock is the frozen clock every Checker in a test shares.
type recheckClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *recheckClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *recheckClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// recheckModel wires a model the way cmd/deck does (a Checker with no
// Enabled gate, so the model's own settings decide) against srv, sharing the
// cache file and the clock. Its recheck tick fires at once.
func recheckModel(srv *recheckServer, clock *recheckClock, cache string, settings config.Settings) Model {
	checker := &upd.Checker{CachePath: cache, Options: upd.Options{BaseURL: srv.URL, Current: "v1.0.0"}, Now: clock.Now}
	m := footerNoteFrameModel(240, 24, false)
	m.settings = settings
	m.updateRecheckAfter = func() time.Duration { return time.Millisecond }
	return m.WithUpdateSource(checker.Check, nil)
}

// startUp delivers the startup check's answer, as Init's command would.
func startUp(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.updateCheckCmd()
	if cmd == nil {
		return m
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

// recheck delivers one recheck tick and runs what it returns: the check's
// answer is applied, and the next tick must be scheduled.
func recheck(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(updateRecheckTick{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("a recheck tick returned no command: the next recheck is not scheduled")
	}
	msgs := []tea.Msg{cmd()}
	if batch, ok := msgs[0].(tea.BatchMsg); ok {
		msgs = msgs[:0]
		for _, c := range batch {
			msgs = append(msgs, c())
		}
	}
	scheduled := false
	for _, msg := range msgs {
		switch msg := msg.(type) {
		case updateRecheckTick:
			scheduled = true
		case updateChecked:
			next, _ = m.Update(msg)
			m = next.(Model)
		default:
			t.Fatalf("unexpected message %T from a recheck", msg)
		}
	}
	if !scheduled {
		t.Fatal("a recheck did not schedule the next one")
	}
	return m
}

const recheckOffer = "update available: v1.1.0 — press ctrl+u to install"

// TestUpdateRecheckShowsReleaseWithoutRestart: two open models share one
// cache. A release published after both started shows in both once the
// interval has passed, with no restart, and only one request is made for it.
func TestUpdateRecheckShowsReleaseWithoutRestart(t *testing.T) {
	srv := newRecheckServer(t, "v1.0.0")
	clock := &recheckClock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	cache := filepath.Join(t.TempDir(), "update-check.json")
	on := config.Settings{UpdateCheck: true}
	a := startUp(t, recheckModel(srv, clock, cache, on))
	b := startUp(t, recheckModel(srv, clock, cache, on))
	if got := srv.checks.Load(); got != 1 {
		t.Fatalf("startup requests = %d, want 1 across both models", got)
	}
	if a.footerNote() != "" || b.footerNote() != "" {
		t.Fatalf("an indicator before any release: %q / %q", a.footerNote(), b.footerNote())
	}

	srv.publish("v1.1.0")
	clock.advance(upd.CheckInterval - time.Minute)
	a, b = recheck(t, a), recheck(t, b)
	if got := srv.checks.Load(); got != 1 {
		t.Fatalf("requests inside the window = %d, want still 1", got)
	}
	if a.footerNote() != "" {
		t.Fatalf("indicator inside the window = %q, want none (the cached answer)", a.footerNote())
	}

	clock.advance(time.Minute)
	a = recheck(t, a)
	if got := a.footerNote(); got != recheckOffer {
		t.Fatalf("model a after the interval = %q, want %q", got, recheckOffer)
	}
	b = recheck(t, b)
	if got := b.footerNote(); got != recheckOffer {
		t.Fatalf("model b, which made no request, = %q, want %q", got, recheckOffer)
	}
	if got := srv.checks.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 (one at startup, one after the interval, across both models)", got)
	}
}

// TestUpdateRecheckOffMakesNoRequest: with update_check and auto_update off
// the recheck tick keeps running but never asks the server.
func TestUpdateRecheckOffMakesNoRequest(t *testing.T) {
	srv := newRecheckServer(t, "v1.1.0")
	clock := &recheckClock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	m := startUp(t, recheckModel(srv, clock, filepath.Join(t.TempDir(), "c.json"), config.Settings{}))
	for range 3 {
		clock.advance(upd.CheckInterval + time.Hour)
		m = recheck(t, m)
	}
	if got := srv.checks.Load(); got != 0 {
		t.Fatalf("requests with the check off = %d, want 0", got)
	}
	if got := m.footerNote(); got != "" {
		t.Fatalf("indicator with the check off = %q", got)
	}
}

// TestUpdateRecheckKeepsHeldRelease: a release an update refused as not
// verified immutable stays held across rechecks (it is not offered again);
// a newer tag is.
func TestUpdateRecheckKeepsHeldRelease(t *testing.T) {
	srv := newRecheckServer(t, "v1.1.0")
	clock := &recheckClock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	m := recheckModel(srv, clock, filepath.Join(t.TempDir(), "c.json"), config.Settings{UpdateCheck: true})
	m.updateInd = updateIndicator{state: updateFailed, tag: "v1.1.0", notImmutable: true}
	held := m.updateInd
	m = recheck(t, m)
	if m.updateInd != held {
		t.Fatalf("held release replaced by %+v", m.updateInd)
	}
	srv.publish("v1.2.0")
	clock.advance(upd.CheckInterval)
	m = recheck(t, m)
	if got := m.updateInd; got.state != updateAvailable || got.tag != "v1.2.0" {
		t.Fatalf("indicator after a newer release = %+v, want v1.2.0 offered", got)
	}
}

// TestUpdateRecheckCmdWithoutSource: a model with no update source schedules
// no recheck, and Init carries none.
func TestUpdateRecheckCmdWithoutSource(t *testing.T) {
	if cmd := footerNoteFrameModel(80, 24, false).updateRecheckCmd(); cmd != nil {
		t.Fatal("a recheck was scheduled without an update source")
	}
}

// TestUpdateRecheckDelay: the production delay is the check interval plus a
// jitter under updateRecheckJitter; a test override replaces it.
func TestUpdateRecheckDelay(t *testing.T) {
	var m Model
	for range 50 {
		if d := m.updateRecheckDelay(); d < upd.CheckInterval || d >= upd.CheckInterval+updateRecheckJitter {
			t.Fatalf("delay %v outside [%v, %v)", d, upd.CheckInterval, upd.CheckInterval+updateRecheckJitter)
		}
	}
	m.updateRecheckAfter = func() time.Duration { return 7 * time.Second }
	if d := m.updateRecheckDelay(); d != 7*time.Second {
		t.Fatalf("override delay = %v", d)
	}
}

// TestUpdateRecheckRetriesAFailedAutoUpdate: with auto_update on, an install
// that failed (for a reason other than immutability) is attempted again at a
// later recheck that still names the release, not before.
func TestUpdateRecheckRetriesAFailedAutoUpdate(t *testing.T) {
	srv := newRecheckServer(t, "v1.1.0")
	clock := &recheckClock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	m := recheckModel(srv, clock, filepath.Join(t.TempDir(), "c.json"), config.Settings{UpdateCheck: true, AutoUpdate: true})
	m.updateInd = updateIndicator{state: updateFailed, tag: "v1.1.0", reason: "disk full"}
	m = recheck(t, m)
	if got := m.updateInd; got.state != updateInstalling || got.tag != "v1.1.0" {
		t.Fatalf("indicator after a recheck = %+v, want v1.1.0 installing again", got)
	}
}
