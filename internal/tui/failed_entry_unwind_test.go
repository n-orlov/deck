package tui

import (
	"context"
	"os/exec"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// splitSideBySideForTest gives a test window horizontal chrome, so
// FitWindowToPane has to compensate for a vertical separator instead of
// landing the requested pane width in a single resize the way it does on a
// single-pane window (internal/tmux's own
// TestFitWindowToPaneOnASinglePaneWindowHasZeroChrome measures that
// zero-chrome case). The new pane runs the same quiet `sleep` command
// newQuietSelectionPane uses and is created with -d, so pane 0 -- the pane
// PreviewPane resolves (tmux.Client.PreviewPane takes Panes[0]) -- stays
// the original one.
func splitSideBySideForTest(t *testing.T, socket, session string) {
	t.Helper()
	args := []string{"-L", socket, "split-window", "-h", "-d", "-t", session, "sleep", "600"}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("split window side by side: %v: %s", err, out)
	}
}

// TestFailedFitUnwindRestoresGeometryAndClearsIsizeRecord covers the one
// failed-entry unwind that has real state behind it: FitWindowToPane
// errors AFTER ResolveIsizeGeometry has already written (or adopted)
// @deck_isize_geometry, and after however many resize-window calls the fit
// loop already made -- FitWindowToPane returns that count precisely
// because its error is not a promise that it changed nothing. A
// release-only unwind there (what enterInteractiveBody did before task
// 112 finished this path) leaves the window partially fitted and the
// geometry record stale behind an entry that refused, so the next entry
// adopts a "pre-entry" size that was really this failed entry's own
// halfway point, and nothing ever restores the window's true original
// size.
//
// The failure is provoked with no test-only branch and no injected fake
// (PRD R8): the requested pane width is chosen so that the fit's
// chrome-compensated resize sequence walks past tmux's own maximum window
// width, which makes resize-window itself fail partway through the loop.
// The premise -- "this really is a fit that fails AFTER at least one
// resize-window landed" -- is measured on an identically shaped twin
// window via a direct FitWindowToPane call, so a future tmux whose limits
// differ fails loudly here instead of silently degrading this test into a
// zero-resize case.
func TestFailedFitUnwindRestoresGeometryAndClearsIsizeRecord(t *testing.T) {
	socket := selectionTestSocket("fitunwind")
	newQuietSelectionPane(t, socket, "deck_fitunwind", 80, 24)
	splitSideBySideForTest(t, socket, "deck_fitunwind")
	newQuietSelectionPane(t, socket, "deck_fitunwindtwin", 80, 24)
	splitSideBySideForTest(t, socket, "deck_fitunwindtwin")

	client := tmux.Client{Socket: socket}
	ctx := context.Background()

	windowTarget, err := tmux.SessionName("fitunwind")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	twinTarget, err := tmux.SessionName("fitunwindtwin")
	if err != nil {
		t.Fatalf("SessionName (twin): %v", err)
	}

	// Pick the model's terminal width from the preview content width the
	// fit will be asked for, rather than the other way round: 6000 content
	// columns is comfortably inside tmux's maximum window width on its
	// own, but the fit's chrome compensation on a side-by-side split
	// overshoots past that maximum after the first couple of resizes.
	const wantContentWidth = 6000
	probe := New(nil, config.Settings{Color: true}, "")
	modelWidth := 0
	for w := wantContentWidth; w <= 3*wantContentWidth; w++ {
		probe.width, probe.height = w, 30
		if cw, ch := probe.previewContentSize(); cw == wantContentWidth && ch >= interactiveMinInnerRows {
			modelWidth = w
			break
		}
	}
	if modelWidth == 0 {
		t.Fatalf("test setup: no terminal width in [%d,%d] yields a %d-column preview content box above the %d-row floor", wantContentWidth, 3*wantContentWidth, wantContentWidth, interactiveMinInnerRows)
	}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = modelWidth, 30
	wantWidth, wantHeight := m.previewContentSize()
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-fitunwind-1", Name: "fitunwind", Slug: "fitunwind", Status: "waiting"}}
	m.selected = rowCursor(0)

	// Premise, measured on the twin: the very fit enterInteractiveBody is
	// about to attempt fails, and fails only after it has already resized
	// the window at least once.
	twinPane, ok, err := client.PreviewPane(ctx, "fitunwindtwin")
	if err != nil || !ok {
		t.Fatalf("PreviewPane (twin): %v (ok=%v)", err, ok)
	}
	resizes, fitErr := client.FitWindowToPane(ctx, twinTarget, twinPane.ID, wantWidth, wantHeight)
	if fitErr == nil {
		t.Fatalf("test assumption violated: fitting %dx%d on the twin succeeded (%d resizes), so enterInteractiveBody's fit bail is never reached", wantWidth, wantHeight, resizes)
	}
	if resizes < 1 {
		t.Fatalf("test assumption violated: the twin's fit failed with %d resize-window calls (%v), so no geometry was left partially fitted and this test could not tell a release-only unwind from a full one", resizes, fitErr)
	}

	geometryBefore, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry before the failed entry: %v", err)
	}
	if _, set := readWindowOwnershipOptionForTest(t, socket, windowTarget); set {
		t.Fatalf("test assumption violated: %s already set before the failed entry", tmux.OwnershipOption)
	}
	if _, set := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); set {
		t.Fatalf("test assumption violated: %s already set before the failed entry", tmux.IsizeGeometryOption)
	}

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if got.interactive {
		t.Fatalf("entry entered interactive mode even though the fit fails")
	}
	if !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalOther {
		t.Fatalf("failed entry did not refuse via the ladder's own entryRefusal (other kind): entryRefusal=%+v attachError=%q", got.entryRefusal, got.attachError)
	}

	geometryAfter, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the failed entry: %v", err)
	}
	if geometryAfter != geometryBefore {
		t.Fatalf("the failed entry left the window resized: before=%+v after=%+v, want the pre-entry geometry restored", geometryBefore, geometryAfter)
	}
	if value, set := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); set {
		t.Fatalf("the failed entry left %s set to %q, want it cleared: the next entry would adopt this failed entry's own halfway size as the window's original geometry", tmux.IsizeGeometryOption, value)
	}
	if value, set := readWindowOwnershipOptionForTest(t, socket, windowTarget); set {
		t.Fatalf("the failed entry left %s set to %q, want the claim released", tmux.OwnershipOption, value)
	}
}
