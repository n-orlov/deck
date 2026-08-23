package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newBareGeometrySession starts a single-window tmux session directly on a
// throwaway socket, at the given size, without Client.Bootstrap -- geometry
// reads/writes are asserted against tmux's own unmodified defaults, the
// same pattern ownership_test.go's newBareOwnershipSession uses.
func newBareGeometrySession(t *testing.T, socket, session string, width, height int) (cleanup func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"-L", socket, "new-session", "-d", "-s", session, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)}
	if output, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start bare tmux session: %v: %s", err, output)
	}
	return func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = exec.CommandContext(closeCtx, "tmux", "-L", socket, "kill-server").Run()
	}
}

func geometrySocket(name string) string {
	return fmt.Sprintf("deck-geometry-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

func runTmux(t *testing.T, socket string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	full := append([]string{"-L", socket}, args...)
	output, err := exec.CommandContext(ctx, "tmux", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux -L %s %s: %v: %s", socket, strings.Join(args, " "), err, output)
	}
	return strings.TrimRight(string(output), "\n")
}

// TestCaptureWindowGeometryReadsDimensionsAndWindowLocalWindowSize proves
// PRD II-7's read: dimensions come back correctly, the window-local
// `window-size` value reads as unset before anything ever sets it
// window-locally (deck's own Bootstrap only ever writes it at the
// server-global scope -- task 028/032's precedent), and setting it
// window-locally afterwards is what CaptureWindowGeometry reports next,
// never the server-global value, proving this is the scope-aware read
// task 028 established, not a merged/effective one.
func TestCaptureWindowGeometryReadsDimensionsAndWindowLocalWindowSize(t *testing.T) {
	socket := geometrySocket("capture")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	geometry, err := client.CaptureWindowGeometry(context.Background(), "s0")
	if err != nil {
		t.Fatalf("capture window geometry: %v", err)
	}
	if geometry.Width != 80 || geometry.Height != 24 {
		t.Fatalf("geometry dimensions = %dx%d, want 80x24", geometry.Width, geometry.Height)
	}
	if geometry.WindowSizeSet {
		t.Fatalf("WindowSizeSet = true before anything ever set window-size window-locally; want unset (value %q)", geometry.WindowSizeValue)
	}

	// A server-global window-size (the only scope deck's own Bootstrap ever
	// writes to, per tmux_contract.feature) must NOT leak into this
	// window-local read.
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	geometry, err = client.CaptureWindowGeometry(context.Background(), "s0")
	if err != nil {
		t.Fatalf("capture window geometry after a server-global window-size write: %v", err)
	}
	if geometry.WindowSizeSet {
		t.Fatalf("WindowSizeSet = true after a server-GLOBAL window-size write; a global write must not leak into the window-local read (value %q)", geometry.WindowSizeValue)
	}

	// Now write it window-locally, the scope CaptureWindowGeometry actually
	// reads and the scope exit (task 035) will need to know about to decide
	// whether entry ever touched it.
	runTmux(t, socket, "set-window-option", "-t", "s0", "window-size", "manual")
	geometry, err = client.CaptureWindowGeometry(context.Background(), "s0")
	if err != nil {
		t.Fatalf("capture window geometry after a window-local window-size write: %v", err)
	}
	if !geometry.WindowSizeSet || geometry.WindowSizeValue != "manual" {
		t.Fatalf("WindowSizeSet=%v WindowSizeValue=%q after a window-local write of \"manual\", want set=true value=\"manual\"", geometry.WindowSizeSet, geometry.WindowSizeValue)
	}
}

// TestFitWindowToPaneOnASinglePaneWindowHasZeroChrome proves the
// degenerate II-8 case named in the PRD text itself: on a single-pane
// window #{pane_height} == #{window_height}, so the very first
// resize-window call lands the target exactly -- exactly one resize, no
// iteration needed.
func TestFitWindowToPaneOnASinglePaneWindowHasZeroChrome(t *testing.T) {
	socket := geometrySocket("nochrome")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	resizes, err := client.FitWindowToPane(context.Background(), "s0", "s0", 45, 22)
	if err != nil {
		t.Fatalf("fit window to pane: %v", err)
	}
	if resizes != 1 {
		t.Fatalf("resizes = %d, want exactly 1 on a single-pane window (zero chrome)", resizes)
	}
	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read pane size after fit: %v", err)
	}
	if paneWidth != 45 || paneHeight != 22 {
		t.Fatalf("pane size after fit = %dx%d, want 45x22", paneWidth, paneHeight)
	}
}

// TestFitWindowToPaneConvergesOnASplitWindow proves PRD II-8's central
// claim on a real split window: chrome (the space a sibling pane and its
// border consume) is PROPORTIONAL to the window's own size, not fixed, so
// naively adding a single fixed offset does not land the target in one
// shot -- but the chrome-compensated loop (re-measuring chrome fresh every
// iteration) converges in a small, bounded number of resize-window calls,
// landing the pane at EXACTLY the wanted size, never merely close.
//
// The setup mirrors the PRD's own II-8 text as closely as this repository
// can reproduce without the unavailable raw spike evidence (see tasks.json
// discovered.prdCorrections): a 42-row window split into a 31-row main
// pane over a 10-row sibling, landing a 22-row main pane. Measured here
// (docs/reports/phase3b.md): 4 resize-window calls, not the PRD-cited 5-6
// -- close but not identical, attributed to a different tmux
// version/layout than the unreachable spike's, not to a different
// algorithm.
func TestFitWindowToPaneConvergesOnASplitWindow(t *testing.T) {
	socket := geometrySocket("split")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 42)
	defer cleanup()
	runTmux(t, socket, "split-window", "-v", "-t", "s0", "-l", "10")
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	mainPane := "s0.0"
	resizes, err := client.FitWindowToPane(context.Background(), "s0", mainPane, 80, 22)
	if err != nil {
		t.Fatalf("fit window to pane on a split window: %v", err)
	}
	if resizes < 1 || resizes > 6 {
		t.Fatalf("resizes = %d, want a small bounded number (measured 4, PRD cites 5-6) -- either shape indicates a regression, not a wider layout difference", resizes)
	}
	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(context.Background(), mainPane)
	if err != nil {
		t.Fatalf("read pane size after fit: %v", err)
	}
	if paneWidth != 80 || paneHeight != 22 {
		t.Fatalf("main pane size after fit = %dx%d, want exactly 80x22 -- \"close\" is not landed", paneWidth, paneHeight)
	}
}

// TestFitWindowToPaneNeverCallsResizePane is the grep-shaped proof PRD
// II-8 demands ("grep proves no resize-pane call on this path") turned
// into a test that fails the build the moment it stops being true, rather
// than a manual check that bit-rots the first time someone edits
// geometry.go without re-running grep by hand.
func TestFitWindowToPaneNeverCallsResizePane(t *testing.T) {
	source, err := os.ReadFile("geometry.go")
	if err != nil {
		t.Fatalf("read geometry.go: %v", err)
	}
	if strings.Contains(string(source), "resize-pane") {
		t.Fatalf("geometry.go names \"resize-pane\"; PRD II-8 requires the WINDOW, never the pane, to be resized on this path")
	}
}

// naivePaneTargetingLoop is the alternative PRD II-8 explicitly rejects,
// reproduced here ONLY to demonstrate why: it treats the WANTED pane
// height as if it were the window height to request, never compensating
// for chrome at all. It is not shipped code -- geometry.go's
// FitWindowToPane is the real implementation -- this function exists
// solely so TestNaivePaneTargetingLoopNeverConverges has something to run
// against.
func naivePaneTargetingLoop(t *testing.T, socket, target, paneTarget string, wantWidth, wantHeight, maxAttempts int) (finalPaneHeight int, converged bool) {
	t.Helper()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(context.Background(), paneTarget)
		if err != nil {
			t.Fatalf("naive loop: read pane size: %v", err)
		}
		if paneWidth == wantWidth && paneHeight == wantHeight {
			return paneHeight, true
		}
		// The naive mistake: request the WANTED pane size as the window
		// size directly, never adding the chrome the sibling pane and its
		// border are consuming.
		if err := client.resizeWindow(context.Background(), target, wantWidth, wantHeight); err != nil {
			t.Fatalf("naive loop: resize-window: %v", err)
		}
	}
	_, _, _, paneHeight, err := client.windowAndPaneSize(context.Background(), paneTarget)
	if err != nil {
		t.Fatalf("naive loop: final read pane size: %v", err)
	}
	return paneHeight, false
}

// TestNaivePaneTargetingLoopNeverConverges proves the negative control PRD
// II-8 names: on the same split-window layout TestFitWindowToPaneConverges
// OnASplitWindow lands exactly, a loop that targets resize-window with the
// wanted PANE size directly (never compensating for chrome) never reaches
// the target at all -- it stabilizes on the wrong pane height within a
// couple of iterations and then sits there for the rest of the bound,
// which is what "stuck" means here. The PRD's own spike cites "12
// iterations, pane_height stuck at 11" for a layout this repository cannot
// re-run (raw evidence unavailable); measured here on the reproducible
// 42-row/10-row-sibling layout, the naive loop is stuck at pane_height 20,
// not the target 22, for the full 15-iteration bound -- a different
// number, the same divergent shape.
func TestNaivePaneTargetingLoopNeverConverges(t *testing.T) {
	socket := geometrySocket("naive")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 42)
	defer cleanup()
	runTmux(t, socket, "split-window", "-v", "-t", "s0", "-l", "10")

	finalHeight, converged := naivePaneTargetingLoop(t, socket, "s0", "s0.0", 80, 22, 15)
	if converged {
		t.Fatalf("naive pane-targeting loop converged to pane_height %d; want it to remain stuck, proving the window-targeted chrome-compensated loop is necessary", finalHeight)
	}
	if finalHeight == 22 {
		t.Fatalf("naive loop's final pane_height is the wanted 22 despite converged=false; test bookkeeping is wrong")
	}
}
