package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R239 (#75): a host WindowSizeMsg re-fits the preview and resizes the
// attached interactive pane with no further input. These tests drive a real
// tmux server through a wire-logging shim and read the window's real size.

const hostResizeSessionID = "sess-hostresize-1"

// newHostResizeModel builds a model on a real 80x24 tmux session at the given
// terminal size, with passive fit on, and returns the wire log with it.
func newHostResizeModel(t *testing.T, slug string, width, height int) (Model, string, string) {
	t.Helper()
	socket := selectionTestSocket(slug)
	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
	binary, wireLog := newTmuxWireLogger(t)
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond, Color: true}, "")
	m.width, m.height = width, height
	m.tmuxClient = tmux.Client{Socket: socket, Binary: binary}
	m.sessions = []store.Session{{ID: hostResizeSessionID, Slug: slug, Name: slug, Status: "running"}}
	m.selected = rowCursor(0)
	return m, socket, wireLog
}

// runFitCmd runs the fit command a resize or tick scheduled and feeds its
// previewFitDone back, the way the event loop would.
func runFitCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a fit command, got none")
	}
	done, ok := cmd().(previewFitDone)
	if !ok {
		t.Fatalf("the fit command did not produce a previewFitDone")
	}
	updated, _ := m.Update(done)
	return updated.(Model)
}

// settleTickFit does what the first previewTick after a selection does.
func settleTickFit(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(previewTick(time.Now()))
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick returned %d commands, want 2 (reschedule plus a fit)", len(cmds))
	}
	return runFitCmd(t, updated.(Model), cmds[1])
}

func resize(m Model, width, height int) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model), cmd
}

func wantPaneAtPreviewBox(t *testing.T, m Model, socket, slug string) {
	t.Helper()
	wantW, wantH := m.previewContentSize()
	if gotW, gotH := paneSizeForTest(t, socket, "deck_"+slug); gotW != wantW || gotH != wantH {
		t.Fatalf("pane is %dx%d, want the preview box %dx%d", gotW, gotH, wantW, wantH)
	}
}

func TestHostResizeGrowRefitsThePreviewWithNoFurtherInput(t *testing.T) {
	const slug = "hostresizegrow"
	m, socket, _ := newHostResizeModel(t, slug, 100, 30)
	m = settleTickFit(t, m)
	wantPaneAtPreviewBox(t, m, socket, slug)

	m, cmd := resize(m, 140, 40)
	m = runFitCmd(t, m, cmd)
	wantPaneAtPreviewBox(t, m, socket, slug)
	if w, _ := paneSizeForTest(t, socket, "deck_"+slug); w <= 60 {
		t.Fatalf("pane width %d did not grow with the terminal", w)
	}
}

func TestHostResizeShrinkRefitsThePreviewWithNoFurtherInput(t *testing.T) {
	const slug = "hostresizeshrink"
	m, socket, _ := newHostResizeModel(t, slug, 140, 40)
	m = settleTickFit(t, m)
	wantPaneAtPreviewBox(t, m, socket, slug)
	grownW, grownH := paneSizeForTest(t, socket, "deck_"+slug)

	m, cmd := resize(m, 100, 30)
	m = runFitCmd(t, m, cmd)
	wantPaneAtPreviewBox(t, m, socket, slug)
	if w, h := paneSizeForTest(t, socket, "deck_"+slug); w >= grownW || h >= grownH {
		t.Fatalf("pane %dx%d did not shrink from %dx%d", w, h, grownW, grownH)
	}
}

func TestHostResizeResizesTheAttachedInteractivePaneAndGrid(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		fromW, fromH, toW, toH int
	}{
		{"grow", 100, 30, 140, 40},
		{"shrink", 140, 40, 100, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slug := "hostresizeint" + tc.name
			m, socket, _ := newHostResizeModel(t, slug, tc.fromW, tc.fromH)
			next, _ := m.enterInteractiveBody(false)
			m = next.(Model)
			if !m.interactive {
				t.Fatalf("did not enter interactive mode: %q", m.attachError)
			}
			t.Cleanup(func() { m.teardownInteractive(t.Context()) })
			wantPaneAtPreviewBox(t, m, socket, slug)

			// No command is run and no key is sent: the update itself resizes.
			m, _ = resize(m, tc.toW, tc.toH)
			if !m.interactive {
				t.Fatal("a resize above the floor left interactive mode")
			}
			wantPaneAtPreviewBox(t, m, socket, slug)
			wantW, wantH := m.previewContentSize()
			if g := m.interactiveGrid.Grid(); g.Width() != wantW || g.Height() != wantH {
				t.Fatalf("grid is %dx%d, want %dx%d", g.Width(), g.Height(), wantW, wantH)
			}
		})
	}
}

func TestHostResizeToTheCurrentSizeIssuesNoTmuxCall(t *testing.T) {
	t.Run("preview", func(t *testing.T) {
		m, _, wireLog := newHostResizeModel(t, "hostresizenoop", 100, 30)
		m = settleTickFit(t, m)
		truncateWireLog(t, wireLog)
		m, cmd := resize(m, 100, 30)
		if cmd != nil {
			t.Fatal("a resize to the current size returned a command")
		}
		if m.previewFitSessionID != hostResizeSessionID {
			t.Fatalf("the fit latch = %q, a no-op resize must leave it set", m.previewFitSessionID)
		}
		if n := countWireCommands(t, wireLog, "resize-window"); n != 0 {
			t.Fatalf("%d resize-window calls for a no-op resize", n)
		}
	})
	t.Run("interactive", func(t *testing.T) {
		m, _, wireLog := newHostResizeModel(t, "hostresizenoopint", 100, 30)
		next, _ := m.enterInteractiveBody(false)
		m = next.(Model)
		if !m.interactive {
			t.Fatalf("did not enter interactive mode: %q", m.attachError)
		}
		t.Cleanup(func() { m.teardownInteractive(t.Context()) })
		truncateWireLog(t, wireLog)
		_, cmd := resize(m, 100, 30)
		if cmd != nil {
			t.Fatal("a resize to the current size returned a command")
		}
		if n := countWireCommands(t, wireLog, "resize-window"); n != 0 {
			t.Fatalf("%d resize-window calls for a no-op resize", n)
		}
		if n := countWireCommands(t, wireLog, "capture-pane"); n != 0 {
			t.Fatalf("%d capture-pane calls (a reseed) for a no-op resize", n)
		}
	})
}

func TestHostResizeUnderATakeoverAppliesWhenItCloses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		open  func(*Model)
		close func(*Model)
	}{
		{"modal", func(m *Model) { m.help = true }, func(m *Model) { m.help = false }},
		{"settings", func(m *Model) { m.settingsOpen = true }, func(m *Model) { m.settingsOpen = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slug := "hostresizeunder" + tc.name
			m, socket, wireLog := newHostResizeModel(t, slug, 100, 30)
			m = settleTickFit(t, m)
			oldW, oldH := paneSizeForTest(t, socket, "deck_"+slug)

			tc.open(&m)
			truncateWireLog(t, wireLog)
			m, cmd := resize(m, 140, 40)
			if cmd != nil {
				t.Fatal("a resize under a takeover issued a fit while the main frame is hidden")
			}
			if w, h := paneSizeForTest(t, socket, "deck_"+slug); w != oldW || h != oldH {
				t.Fatalf("pane resized to %dx%d under a takeover", w, h)
			}
			if n := countWireCommands(t, wireLog, "resize-window"); n != 0 {
				t.Fatalf("%d resize-window calls under a takeover", n)
			}
			// Ticks while it is open still hold the fit.
			updated, tickCmd := m.Update(previewTick(time.Now()))
			m = updated.(Model)
			if got := len(previewTickCmds(t, tickCmd)); got != 1 {
				t.Fatalf("a tick under the takeover returned %d commands, want only the reschedule", got)
			}

			tc.close(&m)
			m = settleTickFit(t, m)
			wantPaneAtPreviewBox(t, m, socket, slug)
		})
	}
}
