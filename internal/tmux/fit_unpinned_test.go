package tmux

import (
	"context"
	"testing"
	"time"
)

// TestFitWindowToPaneUnpinnedHoldsTheSizeWithoutPinningTheWindow is the
// operator-reported crop, in both directions, against real tmux.
//
// The report: select a row (passive preview fits its window to the preview
// panel), press `a`, and the full attach comes up cropped to the preview
// panel's own box instead of filling the terminal -- then stays that way
// until an unrelated interactive-mode exit happens to unset the option.
// The cause is not the fit's SIZE but its side effect: `resize-window`
// writes `window-size manual` into the WINDOW options, which shadow the
// server-global `latest`, so the window can no longer follow ANY client.
//
// Both halves are asserted here, the same contrast
// restore_test.go's own TestReversedRestoreOrderLeavesWindowPinned draws
// for the exit recipe:
//
//  1. plain FitWindowToPane (right for interactive mode, which owns and
//     restores the geometry) leaves the window pinned, and a client
//     attaching at a larger size is ignored entirely -- the crop;
//  2. FitWindowToPaneUnpinned still HOLDS the fitted size while nobody is
//     attached (tmux sizes a window from its clients, and a window with
//     none keeps what resize-window gave it), yet hands the window straight
//     to the next client that attaches.
//
// Property 2's first half matters as much as its second: an unpin that
// also lost the size would break passive preview itself, which exists to
// make the panel's own box the size the agent renders at.
func TestFitWindowToPaneUnpinnedHoldsTheSizeWithoutPinningTheWindow(t *testing.T) {
	socket := geometrySocket("unpinned")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	// The scope deck's own Bootstrap writes (tmux_contract.feature), and the
	// only reason unpinning means anything: the window falls back to THIS.
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// --- Property 1: the crop, exactly as reported. ---
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("plain fit window to pane: %v", err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after a plain fit: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after a plain FitWindowToPane = set=%v value=%q, want set=true value=%q -- if resize-window ever stops writing this, the unpin below is unnecessary and this whole test should go", set, value, "manual")
	}

	const clientCols, clientRows = 100, 40
	croppedTerminal, croppedCmd := attachThroughPTY(t, socket, "s0", clientCols, clientRows)
	defer func() { _ = croppedTerminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)
	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size under the pinned attach: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size with a %dx%d client attached to a window a plain fit pinned = %dx%d, want it stuck at the fitted 45x15 -- this is the operator's crop, and without it there is nothing to fix", clientCols, clientRows, paneWidth, paneHeight)
	}
	detachAndWait(t, croppedTerminal, croppedCmd)

	// --- Property 2a: unpinned, and STILL fitted with nobody attached. ---
	// The window is already 45x15 here (the pinned attach above changed
	// nothing), so this fit converges in ZERO resize-window calls -- which
	// makes it the case an "unpin only if we resized" shortcut would get
	// wrong, leaving the pin the previous half wrote.
	resizes, err := client.FitWindowToPaneUnpinned(ctx, "s0", "s0", 45, 15)
	if err != nil {
		t.Fatalf("unpinned fit window to pane: %v", err)
	}
	if resizes != 0 {
		t.Fatalf("the unpinned fit issued %d resize-window calls, want 0 -- the pane already measured 45x15, so this test is no longer exercising the already-fitted path", resizes)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after the unpinned fit: %v", err)
	} else if set {
		t.Fatalf("window-size after FitWindowToPaneUnpinned = %q, want unset -- the unpin must run even when the fit itself resized nothing", value)
	}
	if _, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, "s0"); err != nil {
		t.Fatalf("read pane size after the unpinned fit: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size after the unpinned fit with nobody attached = %dx%d, want the fitted 45x15 held -- unpinning may not cost passive preview the size it just chose", paneWidth, paneHeight)
	}

	// --- Property 2b: the next client to attach wins. ---
	freeTerminal, freeCmd := attachThroughPTY(t, socket, "s0", clientCols, clientRows)
	defer func() { _ = freeTerminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)
	if _, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, "s0"); err != nil {
		t.Fatalf("read pane size under the unpinned attach: %v", err)
	}
	// One row shorter than the client's own terminal: the client draws
	// tmux's status line outside the window, exactly as restore_test.go's
	// correct-order half documents for the same reason.
	if paneWidth != clientCols || paneHeight != clientRows-1 {
		t.Fatalf("pane size with a %dx%d client attached to an UNPINNED window = %dx%d, want %dx%d (the client's own size, minus its one-row status line) -- `a` must never be cropped by deck's own preview fit", clientCols, clientRows, paneWidth, paneHeight, clientCols, clientRows-1)
	}
	detachAndWait(t, freeTerminal, freeCmd)
}

// TestPinWindowSizeHoldsAWindowThatNeededNoResize covers the gap the unpin
// above opens in interactive mode's own entry: entry used to get its
// `window-size manual` free, as resize-window's side effect, which is
// absent in exactly the case passive fit now makes common -- a window
// ALREADY at the interactive box's size, where FitWindowToPane converges in
// zero resizes and writes nothing at all. PinWindowSize states it instead.
//
// The pin is asserted by its effect, not by re-reading the option deck just
// wrote: a client attaching at a larger size must be ignored, which is the
// held size SPEC §11.9 says interactive mode's grid depends on.
func TestPinWindowSizeHoldsAWindowThatNeededNoResize(t *testing.T) {
	socket := geometrySocket("pin")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Passive preview's own fit, leaving the window unpinned at 45x15.
	if _, err := client.FitWindowToPaneUnpinned(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("unpinned fit: %v", err)
	}
	// Interactive entry now fits to the very same box, so its fit is a
	// no-op -- the case that used to leave nothing pinned.
	resizes, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15)
	if err != nil {
		t.Fatalf("interactive-entry fit: %v", err)
	}
	if resizes != 0 {
		t.Fatalf("the entry fit issued %d resize-window calls, want 0 -- this test only means something when the fit writes no pin of its own", resizes)
	}
	if _, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after a zero-resize entry fit: %v", err)
	} else if set {
		t.Fatalf("window-size is set after a fit that issued no resize-window; the premise of this test (and of PinWindowSize) is that it is not")
	}

	if err := client.PinWindowSize(ctx, "s0"); err != nil {
		t.Fatalf("pin window-size: %v", err)
	}
	if _, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0"); err != nil {
		t.Fatalf("read pane size after pinning: %v", err)
	} else if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size after PinWindowSize = %dx%d, want the untouched 45x15 -- pinning holds a size, it does not change one (and so costs no SIGWINCH)", paneWidth, paneHeight)
	}

	const clientCols, clientRows = 100, 40
	terminal, cmd := attachThroughPTY(t, socket, "s0", clientCols, clientRows)
	defer func() { _ = terminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)
	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size under the attach: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size with a %dx%d client attached to a PINNED window = %dx%d, want the held 45x15 -- interactive mode's grid is drawn against the size it pinned", clientCols, clientRows, paneWidth, paneHeight)
	}
	// And the pin is releasable by the same primitive `a` uses, handing the
	// window to the client that is STILL attached -- the pair, not just the
	// pin. This is also the shape of interactive mode's own
	// attached-client-gated restore (RestoreWindowGeometry), where the
	// unset alone is what hands the size back.
	if err := client.UnpinWindowSize(ctx, "s0"); err != nil {
		t.Fatalf("unpin window-size: %v", err)
	}
	waitForPaneSize(t, client, "s0", clientCols, clientRows-1)
	detachAndWait(t, terminal, cmd)
}

// waitForPaneSize polls target's #{pane_width}x#{pane_height} until it
// reads wantWidth x wantHeight or a deadline expires, the same shape
// waitForSessionAttachedCount uses. Measured here, tmux completes this
// particular resize inside the option write itself (the first poll reads
// the new size), so the loop is tolerance for a slower machine, not a
// known asynchrony -- an immediate read would pass today.
func waitForPaneSize(t *testing.T, client Client, target string, wantWidth, wantHeight int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastWidth, lastHeight int
	var lastErr error
	for time.Now().Before(deadline) {
		_, _, lastWidth, lastHeight, lastErr = client.windowAndPaneSize(context.Background(), target)
		if lastErr == nil && lastWidth == wantWidth && lastHeight == wantHeight {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pane %q never reached %dx%d within 3s (last read %dx%d, err %v)", target, wantWidth, wantHeight, lastWidth, lastHeight, lastErr)
}
