package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R63: a passive preview fit can never overlap itself. previewFitSessionID
// is only written when an attempt COMPLETES (previewFitDone), so on its own
// it cannot coalesce two previewTicks that both fire inside one
// PreviewPane+FitWindowToPane round trip -- both saw the selection as
// unsettled, both issued a fit for the SAME session, and the window was
// resized twice, costing a second SIGWINCH the scenarios counting them
// (features/preview.feature's "exactly 1") never asked for.
// previewFitInFlight, set at SCHEDULING time, closes that window.
//
// All three tests below drive Model.Update directly -- no pty, no tmux, no
// socket -- and assert on the tea.Cmd Update RETURNED, never on the fit
// closure's effects: the closure is deliberately never run.

// previewFitModel builds the smallest model for which previewFit issues a
// command: fit enabled, a tmux socket named (never dialled), a valid
// selection, and a frame whose preview panel is shown and above the
// interactive inner-row floor.
func previewFitModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.sessions = []store.Session{
		{ID: "s1", Slug: "one", Name: "one", Status: "running"},
		{ID: "s2", Slug: "two", Name: "two", Status: "running"},
	}
	m.selected = 0
	m.width, m.height = 100, 30
	// Never dialled: no test here runs the fit closure, so this only has to
	// be non-empty to pass previewFit's "is there a tmux client wired" guard.
	m.tmuxClient = tmux.Client{Socket: "/nonexistent/deck-r63-test.sock"}
	// previewCapture is left nil on purpose: with no capture engine wired the
	// only commands a previewTick can batch are the unconditional reschedule
	// and the passive fit, which is what previewTickCmds counts.
	if !m.computeLayout().PreviewShown {
		t.Fatal("test setup: PreviewShown = false at 100x30, want true")
	}
	if w, h := m.previewContentSize(); w <= 0 || h < interactiveMinInnerRows {
		t.Fatalf("test setup: preview content box = %dx%d, want width > 0 and height >= %d", w, h, interactiveMinInnerRows)
	}
	return m
}

// previewTickCmds flattens what Update returned for a previewTick into the
// commands the real event loop would run. tea.Batch collapses a single valid
// command to that command itself, so a tick that issued NO fit returns the
// bare reschedule rather than a one-member tea.BatchMsg; running it yields a
// previewTick message (after this model's 1ms tick interval) instead of a
// tea.BatchMsg, which is how the two cases are told apart.
func previewTickCmds(t *testing.T, cmd tea.Cmd) []tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("previewTick returned no command at all; the reschedule is unconditional")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	if _, ok := msg.(previewTick); !ok {
		t.Fatalf("previewTick returned a single command producing %T, want either a tea.BatchMsg or the reschedule's previewTick", msg)
	}
	return []tea.Cmd{cmd}
}

// TestPreviewFitDoesNotOverlapItself is R63's regression test: the second of
// two previewTicks with NO previewFitDone in between must issue no fit.
func TestPreviewFitDoesNotOverlapItself(t *testing.T) {
	m := previewFitModel(t)

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("first previewTick returned %d commands, want 2 (the reschedule plus one passive fit)", len(got))
	}
	if m.previewFitInFlight != "s1" {
		t.Fatalf("previewFitInFlight = %q after scheduling a fit for s1, want %q -- the marker must be set at scheduling time, not when the fit completes", m.previewFitInFlight, "s1")
	}
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q before any previewFitDone landed, want empty", m.previewFitSessionID)
	}

	// Second tick, first fit still running (no previewFitDone delivered).
	updated, cmd = m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 1 {
		t.Fatalf("second previewTick with the first fit still in flight returned %d commands, want 1 (the reschedule alone): a second overlapping fit resizes the same window again and costs an extra SIGWINCH", len(got))
	}
	if m.previewFitInFlight != "s1" {
		t.Fatalf("previewFitInFlight = %q after the refused second tick, want the still-outstanding %q", m.previewFitInFlight, "s1")
	}
}

// TestPreviewFitResumesAfterItsDoneLands proves the guard is a coalescer and
// not a latch: once the first attempt reports, a changed selection fits again
// on the very next tick.
func TestPreviewFitResumesAfterItsDoneLands(t *testing.T) {
	m := previewFitModel(t)

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("first previewTick returned %d commands, want 2 (the reschedule plus one passive fit)", len(got))
	}

	// The first fit reports, exactly as its closure's every return path does.
	updated, _ = m.Update(previewFitDone{sessionID: "s1"})
	m = updated.(Model)
	if m.previewFitInFlight != "" || m.previewFitSessionID != "s1" {
		t.Fatalf("after previewFitDone{s1}: previewFitInFlight = %q previewFitSessionID = %q, want (\"\", %q)", m.previewFitInFlight, m.previewFitSessionID, "s1")
	}

	// The selection settles somewhere else, so this tick owes a real fit.
	m.selected = 1
	updated, cmd = m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("previewTick after the first fit reported and the selection changed returned %d commands, want 2 (the reschedule plus a fit for the new selection)", len(got))
	}
	if m.previewFitInFlight != "s2" {
		t.Fatalf("previewFitInFlight = %q, want %q -- the new selection's fit is the one now outstanding", m.previewFitInFlight, "s2")
	}
}

// TestPreviewFitDoneForUnselectedSessionClearsTheMarker covers the case that
// would wedge passive fit outright: the user keeps navigating while a fit
// runs, so the previewFitDone that lands names a session that is no longer
// selected. It must still clear the in-flight marker, leaving the next tick
// free to fit the row now selected.
func TestPreviewFitDoneForUnselectedSessionClearsTheMarker(t *testing.T) {
	m := previewFitModel(t)

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("first previewTick returned %d commands, want 2 (the reschedule plus one passive fit)", len(got))
	}

	// Navigate away while s1's fit is still running, then let it report.
	m.selected = 1
	updated, _ = m.Update(previewFitDone{sessionID: "s1"})
	m = updated.(Model)
	if m.previewFitInFlight != "" {
		t.Fatalf("previewFitInFlight = %q after a previewFitDone for the no-longer-selected s1, want cleared -- otherwise passive fit is wedged for good", m.previewFitInFlight)
	}

	updated, cmd = m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("previewTick after a stale previewFitDone returned %d commands, want 2 (the reschedule plus a fit for the row now selected)", len(got))
	}
	if m.previewFitInFlight != "s2" {
		t.Fatalf("previewFitInFlight = %q, want %q", m.previewFitInFlight, "s2")
	}
}
