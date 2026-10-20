// resize_sites_instant_test.go is R411 cases 1-3: the three sites where deck
// resizes a session's window (the passive preview fit, entering the
// interactive preview and leaving it) each take their discount instant BEFORE
// their first resizing tmux call. The discount span starts at that instant
// (R410), so an instant taken after the call would leave the pane's
// resize-driven repaint outside it. The tmux the sites talk to is a real tmux
// behind a wrapper that stamps the moment each resize-window reaches it.
package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// instantDiscounter records every discount with the instant it was given.
type instantDiscounter struct {
	mu   sync.Mutex
	seen []instantDiscount
}

type instantDiscount struct {
	id    string
	cause service.DiscountCause
	at    time.Time
}

func (d *instantDiscounter) Discount(id string, cause service.DiscountCause, at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, instantDiscount{id, cause, at})
}
func (d *instantDiscounter) AttachStarted(string, time.Time) {}
func (d *instantDiscounter) AttachEnded(string, time.Time)   {}

// resizes returns the instants of the DiscountResize discounts for id.
func (d *instantDiscounter) resizes(id string) []time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []time.Time
	for _, s := range d.seen {
		if s.id == id && s.cause == service.DiscountResize {
			out = append(out, s.at)
		}
	}
	return out
}

func (d *instantDiscounter) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = nil
}

// resizeStampingTmux returns a tmux binary that appends the wall-clock
// nanoseconds to log each time it is asked for a resize-window, then runs the
// real tmux unchanged, plus a reader of the stamps taken so far.
func resizeStampingTmux(t *testing.T) (binary string, stamps func() []int64, forget func()) {
	t.Helper()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "resizes")
	binary = filepath.Join(dir, "tmux")
	body := "#!/bin/sh\nfor arg in \"$@\"; do\n\tcase \"$arg\" in\n\tresize-window) date +%s%N >> '" + log + "' ;;\n\tesac\ndone\nexec '" + tmuxPath + "' \"$@\"\n"
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	stamps = func() []int64 {
		raw, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out []int64
		for _, f := range strings.Fields(string(raw)) {
			n, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				t.Fatalf("resize stamp %q: %v", f, err)
			}
			out = append(out, n)
		}
		return out
	}
	forget = func() { _ = os.Remove(log) }
	return binary, stamps, forget
}

// requireInstantBeforeFirstResize fails unless the discounter saw a
// DiscountResize for id whose instant is no later than the first resize the
// tmux wrapper stamped.
func requireInstantBeforeFirstResize(t *testing.T, rec *instantDiscounter, id string, stamps []int64) {
	t.Helper()
	if len(stamps) == 0 {
		t.Fatal("test setup: tmux received no resize-window, so there is no call to precede")
	}
	ats := rec.resizes(id)
	if len(ats) == 0 {
		t.Fatalf("no DiscountResize reached the output clock for %s", id)
	}
	if first := ats[0].UnixNano(); first > stamps[0] {
		t.Fatalf("DiscountResize instant is %d ns AFTER tmux received its first resize-window; it must be taken before the call", first-stamps[0])
	}
}

func TestResizeSitesDiscountFromBeforeTheCall(t *testing.T) {
	newModel := func(t *testing.T, name string, settings config.Settings) (Model, *instantDiscounter, func() []int64, func(), string) {
		t.Helper()
		socket := selectionTestSocket(name)
		slug := "rsd" + name
		newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
		binary, stamps, forget := resizeStampingTmux(t)
		rec := &instantDiscounter{}
		m := New(nil, settings, "").WithOutputDiscounter(rec)
		m.width, m.height = 100, 30
		m.tmuxClient = tmux.Client{Binary: binary, Socket: socket}
		id := "sess-" + slug
		m.sessions = []store.Session{{ID: id, Slug: slug, Name: slug, Status: "waiting"}}
		m.selected = rowCursor(0)
		return m, rec, stamps, forget, id
	}

	t.Run("passive-preview-fit", func(t *testing.T) {
		m, rec, stamps, _, id := newModel(t, "fit", config.Settings{PreviewFit: true, Preview: time.Millisecond})
		if w, h := m.previewContentSize(); w == 80 && h == 24 {
			t.Fatal("test setup: the panel box equals the pane size, so the fit would not resize")
		}
		_, cmd := m.Update(previewTick(time.Now()))
		cmds := previewTickCmds(t, cmd)
		if len(cmds) != 2 {
			t.Fatalf("previewTick returned %d commands, want 2", len(cmds))
		}
		if _, ok := cmds[1]().(previewFitDone); !ok {
			t.Fatal("the fit command did not produce previewFitDone")
		}
		requireInstantBeforeFirstResize(t, rec, id, stamps())
	})

	t.Run("interactive-entry", func(t *testing.T) {
		m, rec, stamps, _, id := newModel(t, "entry", config.Settings{Color: true})
		next, _ := m.enterInteractive()
		got := next.(Model)
		if !got.interactive {
			t.Fatalf("enterInteractive did not enter interactive mode: %q", got.attachError)
		}
		t.Cleanup(func() { got.exitInteractive() })
		requireInstantBeforeFirstResize(t, rec, id, stamps())
	})

	t.Run("interactive-exit", func(t *testing.T) {
		m, rec, stamps, forget, id := newModel(t, "exit", config.Settings{Color: true})
		next, _ := m.enterInteractive()
		got := next.(Model)
		if !got.interactive {
			t.Fatalf("enterInteractive did not enter interactive mode: %q", got.attachError)
		}
		// Only the exit's own resizes and discounts are under test.
		forget()
		rec.reset()
		got.exitInteractive()
		requireInstantBeforeFirstResize(t, rec, id, stamps())
	})
}
