package tui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestContentionRefusalsNameFFloorAndNoWidthRefusalsDoNot is task 106's own
// backstop: PRD/SPEC requirement 47's two CONTENTION refusals (another
// client already attached to the session; a live process already holding
// the window's ownership option) are exactly the two `F` (task 105) exists
// to let a user route around, so both messages must now say so alongside
// the existing "press a to attach" offer -- while the other two refusal
// messages in the same ladder (the 7-row floor; no width at all, checked
// even before the floor) are not offers `F` can do anything about (no
// window is ever touched on either path), so they must keep naming only
// `a` and never mention `F`.
func TestContentionRefusalsNameFFloorAndNoWidthRefusalsDoNot(t *testing.T) {
	// Task 007/R143 (GH #38) moved the way-out text off attachError's
	// string onto entryRefusalWayOut(kind), rendered in the §11.9 banner's
	// own line 3 rather than the footer -- these four checks now read that
	// function's output for each refusal's own kind instead of a raw
	// attachError string.
	t.Run("attached client", func(t *testing.T) {
		got := attachedClientRefusalMessage(t)
		if !strings.Contains(got, "F") {
			t.Fatalf("attached-client refusal way-out %q does not name F", got)
		}
		if !strings.Contains(got, "a attaches instead") {
			t.Fatalf("attached-client refusal way-out %q lost the a-attaches-instead offer", got)
		}
	})

	t.Run("live ownership", func(t *testing.T) {
		got := liveOwnershipRefusalMessage(t)
		if !strings.Contains(got, "F") {
			t.Fatalf("live-ownership refusal way-out %q does not name F", got)
		}
		if !strings.Contains(got, "a attaches instead") {
			t.Fatalf("live-ownership refusal way-out %q lost the a-attaches-instead offer", got)
		}
	})

	t.Run("7-row floor", func(t *testing.T) {
		got := floorRefusalMessage(t)
		if strings.Contains(got, "F") {
			t.Fatalf("floor refusal way-out %q offers F, but F cannot do anything about a squeezed preview box", got)
		}
		if !strings.Contains(got, "a attaches instead") {
			t.Fatalf("floor refusal way-out %q lost the a-attaches-instead offer", got)
		}
	})

	t.Run("no width", func(t *testing.T) {
		got := noWidthRefusalMessage(t)
		if strings.Contains(got, "F") {
			t.Fatalf("no-width refusal way-out %q offers F, but F cannot do anything about a preview panel with no width at all", got)
		}
		if strings.Contains(got, "a attaches instead") {
			t.Fatalf("no-width refusal way-out %q offers a, but the \"other\" kind names neither key", got)
		}
	})
}

// attachedClientRefusalMessage reproduces TestForceEntersDespiteAnAttachedClient's
// own real-tmux setup (a real client attached through a pty) just far
// enough to read the plain (force=false) refusal's attachError.
func attachedClientRefusalMessage(t *testing.T) string {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("fnameattached")
	newQuietSelectionPane(t, socket, "deck_fnameattached", 80, 24)
	client := tmux.Client{Socket: socket}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-fname-1", Name: "fnameattached", Slug: "fnameattached", Status: "waiting"}}
	m.selected = rowCursor(0)

	windowTarget, err := tmux.SessionName("fnameattached")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive (force=false) entered while a real client was attached")
	}
	return entryRefusalWayOut(got.entryRefusal.kind)
}

// liveOwnershipRefusalMessage reproduces
// features/interactive_refusals_test.go's own live-ownership setup (a raw
// tmux `set-option` naming this test binary's own live pid) just far
// enough to read the plain refusal's attachError.
func liveOwnershipRefusalMessage(t *testing.T) string {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("fnameowned")
	newQuietSelectionPane(t, socket, "deck_fnameowned", 80, 24)
	client := tmux.Client{Socket: socket}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-fname-2", Name: "fnameowned", Slug: "fnameowned", Status: "waiting"}}
	m.selected = rowCursor(0)

	windowTarget, err := tmux.SessionName("fnameowned")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	claim := "other-owner-claim:" + strconv.Itoa(os.Getpid())
	cmd := exec.CommandContext(context.Background(), "tmux", "-L", socket, "set-option", "-w", "-t", windowTarget, "@deck_isize_owner", claim)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux -L %s set-option -w -t %s @deck_isize_owner %s: %v: %s", socket, windowTarget, claim, err, out)
	}

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered despite a live ownership holder")
	}
	return entryRefusalWayOut(got.entryRefusal.kind)
}

// floorRefusalMessage reuses
// TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall's own
// no-tmux-server fixture: the floor check runs before any tmux call, so no
// real tmux server is needed to read this refusal's attachError.
func floorRefusalMessage(t *testing.T) string {
	t.Helper()
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-106"})
	m.sessions = []store.Session{{ID: "s1", Name: "squeezed", Agent: "shell", Status: "running", Slug: "squeezed"}}
	m.selected = rowCursor(0)
	m.width, m.height = 80, 9

	if _, height := m.previewContentSize(); height >= interactiveMinInnerRows {
		t.Fatalf("fixture height is not below interactiveMinInnerRows (%d); this test would be vacuous", interactiveMinInnerRows)
	}

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode below the %d-row floor", interactiveMinInnerRows)
	}
	return entryRefusalWayOut(got.entryRefusal.kind)
}

// noWidthRefusalMessage drives previewContentSize's width<=0 branch (the
// refusal checked even before the 7-row floor) with a terminal narrow
// enough that the stacked layout's own preview width (equal to the full
// terminal width) minus the 4-column content inset is zero or negative --
// no tmux call is reachable on this path either.
func noWidthRefusalMessage(t *testing.T) string {
	t.Helper()
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-106-width"})
	m.sessions = []store.Session{{ID: "s1", Name: "narrow", Agent: "shell", Status: "running", Slug: "narrow"}}
	m.selected = rowCursor(0)
	m.width, m.height = 3, 30

	width, _ := m.previewContentSize()
	if width > 0 {
		t.Fatalf("fixture width %d is not <= 0; this test would be vacuous", width)
	}

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode with no preview width at all")
	}
	return entryRefusalWayOut(got.entryRefusal.kind)
}
