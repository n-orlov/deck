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

// runCmds runs a command the way the event loop would -- flattening a
// tea.Batch -- and returns every message it produced.
func runCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmds(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// feedFitDone runs cmd and feeds the previewFitDone it yields back into the
// model; it returns false when no fit was scheduled. It never injects a
// previewTick.
func feedFitDone(t *testing.T, m Model, cmd tea.Cmd) (Model, bool) {
	t.Helper()
	for _, msg := range runCmds(cmd) {
		if done, ok := msg.(previewFitDone); ok {
			updated, _ := m.Update(done)
			return updated.(Model), true
		}
	}
	return m, false
}

func keyPress(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(key)
	return updated.(Model), cmd
}

var (
	keyEsc      = tea.KeyMsg{Type: tea.KeyEscape}
	keyQuestion = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	keyI        = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
)

// takeoverCases open a covering screen on the model and close it through the
// screen's own key path (Update), never by clearing a field.
var takeoverCases = []struct {
	name  string
	open  func(*Model)
	close tea.KeyMsg
}{
	{"help", func(m *Model) { m.help = true }, keyQuestion},
	{"help-escape", func(m *Model) { m.help = true }, keyEsc},
	{"settings", func(m *Model) { m.settingsOpen = true }, keyEsc},
	{"detail", func(m *Model) { m.detail = true }, keyI},
	{"event-log", func(m *Model) { m.eventLogOpen = true }, keyEsc},
	{"delete-confirm", func(m *Model) { m.deleteConfirming = true }, keyEsc},
	{"archive-confirm", func(m *Model) { m.archiveConfirming = true }, keyEsc},
}

func TestHostResizeUnderATakeoverAppliesTheMomentItCloses(t *testing.T) {
	for _, tc := range takeoverCases {
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

			// The closing key itself schedules the fit: no tick, no other event.
			m, cmd = keyPress(m, tc.close)
			if m.takeoverActive() {
				t.Fatalf("key %q did not close the %s screen", tc.close.String(), tc.name)
			}
			m, ok := feedFitDone(t, m, cmd)
			if !ok {
				t.Fatalf("closing the %s screen scheduled no fit", tc.name)
			}
			wantPaneAtPreviewBox(t, m, socket, slug)
			if w, _ := paneSizeForTest(t, socket, "deck_"+slug); w <= oldW {
				t.Fatalf("pane width %d did not grow past %d", w, oldW)
			}

			// Applied once: a further event schedules nothing and calls tmux not at all.
			truncateWireLog(t, wireLog)
			m, cmd = keyPress(m, tea.KeyMsg{Type: tea.KeyDown})
			if _, again := feedFitDone(t, m, cmd); again {
				t.Fatal("the held resize was applied twice")
			}
			if n := countWireCommands(t, wireLog, "resize-window"); n != 0 {
				t.Fatalf("%d resize-window calls after the held resize was applied", n)
			}
		})
	}
}

func TestHostResizeHeldAcrossStackedTakeoversAppliesWhenTheLastCloses(t *testing.T) {
	const slug = "hostresizestacked"
	m, socket, _ := newHostResizeModel(t, slug, 100, 30)
	m = settleTickFit(t, m)
	m.help, m.detail = true, true
	m, _ = resize(m, 140, 40)
	// Opening the event log over them as well, then closing it, leaves the
	// help/detail pair still covering the preview.
	m.eventLogOpen = true
	m, cmd := keyPress(m, keyEsc)
	if _, ok := feedFitDone(t, m, cmd); ok {
		t.Fatal("a fit was scheduled while another screen still covers the preview")
	}
	if !m.previewRefitPending {
		t.Fatal("the held resize was dropped while a screen still covers the preview")
	}
	m, cmd = keyPress(m, keyEsc) // help's Cancel clears help and detail together
	m, ok := feedFitDone(t, m, cmd)
	if !ok {
		t.Fatal("closing the last covering screen scheduled no fit")
	}
	wantPaneAtPreviewBox(t, m, socket, slug)
}

func TestHostResizeHeldBehindAFitInFlightAppliesWhenThatFitLands(t *testing.T) {
	const slug = "hostresizeinflight"
	m, socket, _ := newHostResizeModel(t, slug, 100, 30)
	// A fit for the old size is outstanding while the screen opens, the
	// resize arrives and the screen closes.
	updated, tickCmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, tickCmd)
	if len(cmds) != 2 || m.previewFitInFlight == "" {
		t.Fatalf("expected a fit in flight, got %d commands, in flight %q", len(cmds), m.previewFitInFlight)
	}
	staleFit := cmds[1]
	m.help = true
	m, _ = resize(m, 140, 40)
	m, cmd := keyPress(m, keyQuestion)
	if _, ok := feedFitDone(t, m, cmd); ok {
		t.Fatal("a second fit was scheduled while one is in flight")
	}
	if m.previewFitInFlight == "" {
		t.Fatal("the in-flight marker was cleared by the screen closing")
	}
	// The outstanding fit lands (it fitted the pane to whatever the box was
	// when it ran); landing it schedules the held re-fit with no tick.
	done := runCmds(staleFit)
	updated, cmd = m.Update(done[0])
	m = updated.(Model)
	m, ok := feedFitDone(t, m, cmd)
	if !ok {
		t.Fatal("the fit landing did not apply the held resize")
	}
	wantPaneAtPreviewBox(t, m, socket, slug)
}

func TestHostResizeUnderATakeoverBelowTheFloorOrUnchangedSchedulesNothing(t *testing.T) {
	t.Run("below the floor", func(t *testing.T) {
		const slug = "hostresizebelowfloor"
		m, socket, wireLog := newHostResizeModel(t, slug, 100, 30)
		m = settleTickFit(t, m)
		oldW, oldH := paneSizeForTest(t, socket, "deck_"+slug)
		m.help = true
		m, _ = resize(m, 100, 9)
		truncateWireLog(t, wireLog)
		m, cmd := keyPress(m, keyQuestion)
		if _, ok := feedFitDone(t, m, cmd); ok {
			t.Fatal("a fit was scheduled for a box below the 7-row floor")
		}
		if m.previewRefitPending {
			t.Fatal("the held resize stayed pending after the screen closed")
		}
		if w, h := paneSizeForTest(t, socket, "deck_"+slug); w != oldW || h != oldH {
			t.Fatalf("pane went %dx%d -> %dx%d below the floor", oldW, oldH, w, h)
		}
	})
	t.Run("unchanged size", func(t *testing.T) {
		const slug = "hostresizeunchangedunder"
		m, _, wireLog := newHostResizeModel(t, slug, 100, 30)
		m = settleTickFit(t, m)
		m.help = true
		m, _ = resize(m, 100, 30)
		if m.previewRefitPending {
			t.Fatal("a same-size resize armed a re-fit")
		}
		truncateWireLog(t, wireLog)
		m, cmd := keyPress(m, keyQuestion)
		if _, ok := feedFitDone(t, m, cmd); ok {
			t.Fatal("closing a screen after a same-size resize scheduled a fit")
		}
		if n := countWireCommands(t, wireLog, "resize-window"); n != 0 {
			t.Fatalf("%d resize-window calls for a same-size resize", n)
		}
	})
}
