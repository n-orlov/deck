package tmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"
)

// restoreSocket returns a throwaway socket name for one restore_test.go
// test, the same shape geometrySocket/ownershipSocket already use in their
// own files.
func restoreSocket(name string) string {
	return fmt.Sprintf("deck-restore-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// TestRestoreWindowGeometryDetachedResizesThenUnsets proves PRD II-9's
// recipe in the ordinary case: with nobody attached, exit must actively
// resize-window back to the saved dimensions (nothing else would restore
// them -- the server-global window-size stays "latest" throughout, per
// task 032/034, and "latest" with zero attached clients does nothing),
// THEN unset window-size so a client attaching later is governed by its
// own size rather than staying pinned at whatever entry set.
func TestRestoreWindowGeometryDetachedResizesThenUnsets(t *testing.T) {
	socket := restoreSocket("detached")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	original, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}
	if original.WindowSizeSet {
		t.Fatalf("window-size already set window-locally before entry (value %q); test assumption violated", original.WindowSizeValue)
	}

	// "Enter interactive mode": fit the (single-pane, zero-chrome) window
	// to a different size, which sets window-size=manual as a tmux side
	// effect of resize-window -- exactly what exit's restore has to undo.
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("fit window to pane: %v", err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after fit: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after fit = set=%v value=%q, want set=true value=\"manual\" (test assumption violated, not what this test is proving)", set, value)
	}

	if attached, err := client.SessionAttachedCount(ctx, "s0"); err != nil {
		t.Fatalf("session_attached: %v", err)
	} else if attached != 0 {
		t.Fatalf("session_attached = %d before any client attaches, want 0", attached)
	}

	if err := client.RestoreWindowGeometry(ctx, "s0", original); err != nil {
		t.Fatalf("restore window geometry: %v", err)
	}

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after restore: %v", err)
	}
	if paneWidth != original.Width || paneHeight != original.Height {
		t.Fatalf("pane size after restore = %dx%d, want the original %dx%d", paneWidth, paneHeight, original.Width, original.Height)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after restore: %v", err)
	} else if set {
		t.Fatalf("window-size after restore = set=true value=%q, want unset -- a live client attaching next must be governed by its own size, not left pinned", value)
	}
}

// TestReversedRestoreOrderLeavesWindowPinned demonstrates, red, exactly the
// failure PRD II-9 names by name: "Reversed, the resize re-flips manual
// and the window stays pinned." It issues the two restore steps in the
// WRONG order (unset first, resize-window second -- the reverse of
// RestoreWindowGeometry, which this test deliberately does not call for
// its first half) and shows two things a caller could otherwise miss:
//  1. window-size ends up set to "manual" again, not unset, because
//     resize-window always writes that regardless of which value the
//     unset just restored.
//  2. a client that then attaches at a THIRD size is not followed at all
//     -- the window stays at whatever the reversed sequence's
//     resize-window last set, proving "pinned" is a real, observable
//     behaviour and not just leftover option state nobody reads.
//
// It then runs the CORRECT RestoreWindowGeometry with that same client
// still attached and shows the contrast: only the correct order lets the
// unset alone hand the window straight to the attached client's own size.
func TestReversedRestoreOrderLeavesWindowPinned(t *testing.T) {
	socket := restoreSocket("reversed")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	original, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("fit window to pane: %v", err)
	}

	// The reversed order: unset first, resize-window second -- exactly the
	// mistake PRD II-9 rejects. Called directly, never through
	// RestoreWindowGeometry, which never issues this order.
	if err := client.unsetWindowSize(ctx, "s0"); err != nil {
		t.Fatalf("unset window-size (reversed step 1): %v", err)
	}
	if err := client.resizeWindow(ctx, "s0", original.Width, original.Height); err != nil {
		t.Fatalf("resize-window (reversed step 2): %v", err)
	}

	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after reversed order: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after the REVERSED restore order = set=%v value=%q, want set=true value=\"manual\" -- resize-window re-flips it, undoing the unset that came before it", set, value)
	}

	// A client attaching now, at a THIRD size distinct from both the
	// interactive size (45x15) and the original (80x24), must NOT be
	// followed: window-size is pinned at "manual", so tmux ignores this
	// client's own dimensions entirely.
	const pinnedClientCols, pinnedClientRows = 100, 40
	pinnedTerminal, pinnedCmd := attachThroughPTY(t, socket, "s0", pinnedClientCols, pinnedClientRows)
	defer func() { _ = pinnedTerminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after pinned attach: %v", err)
	}
	if paneWidth != original.Width || paneHeight != original.Height {
		t.Fatalf("pane size after attaching a client at %dx%d while window-size=manual = %dx%d, want it to stay PINNED at the reversed order's %dx%d, ignoring the attaching client entirely", pinnedClientCols, pinnedClientRows, paneWidth, paneHeight, original.Width, original.Height)
	}
	detachAndWait(t, pinnedTerminal, pinnedCmd)

	// Now the contrast: attach a SECOND client (interactive mode again,
	// then the CORRECT restore) and show that this time, with the correct
	// order, the unset alone hands the window straight to the attached
	// client's own size.
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("re-fit window to pane before the correct-order half: %v", err)
	}
	const correctClientCols, correctClientRows = 90, 30
	correctTerminal, correctCmd := attachThroughPTY(t, socket, "s0", correctClientCols, correctClientRows)
	defer func() { _ = correctTerminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)

	if err := client.RestoreWindowGeometry(ctx, "s0", original); err != nil {
		t.Fatalf("correct-order restore with a client attached: %v", err)
	}
	_, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after correct-order restore: %v", err)
	}
	// The attached client's own terminal is correctClientCols x
	// correctClientRows, but the pane it sees is one row shorter: tmux's
	// status line (on by default, one row) is drawn by the client and
	// subtracted from its own screen before "window-size latest" sizes the
	// window to fit -- not a chrome RestoreWindowGeometry itself owns or
	// needs to compensate for (task 034's II-8 chrome loop is about
	// SIBLING panes inside the window; this is the client's own status
	// line, outside the window entirely).
	wantWidth, wantHeight := correctClientCols, correctClientRows-1
	if paneWidth != wantWidth || paneHeight != wantHeight {
		t.Fatalf("pane size after the CORRECT restore order with a client attached = %dx%d, want %dx%d (the attached client's own %dx%d, minus its one-row status line) -- the unset alone should hand control straight back to it", paneWidth, paneHeight, wantWidth, wantHeight, correctClientCols, correctClientRows)
	}
	detachAndWait(t, correctTerminal, correctCmd)
}

// TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch is
// the literal PRD II-9 claim this task's successCriteria names: "with a
// client attached the unset alone restores the size immediately with no
// third SIGWINCH." The pane runs a POSIX shell trapping SIGWINCH into a
// plain append-only counter file -- a real kernel signal, not a size log
// -- so "no third SIGWINCH" is a real count, not an inference from pane
// size alone.
func TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch(t *testing.T) {
	socket := restoreSocket("sigwinch")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })

	winchLog := filepath.Join(t.TempDir(), "winch.count")
	script := `trap 'printf x >> "$WINCH_LOG"' WINCH; while :; do sleep 0.05; done`
	if _, err := client.Create(context.Background(), Launch{
		Slug: "sigwinch", CWD: t.TempDir(),
		Command: []string{"sh", "-c", script},
		Env:     map[string]string{"WINCH_LOG": winchLog},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	target := "deck_sigwinch"
	ctx := context.Background()

	original, err := client.CaptureWindowGeometry(ctx, target)
	if err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}
	if original.Width == 45 && original.Height == 15 {
		t.Fatalf("original geometry %dx%d collides with this test's chosen interactive size; pick a different interactive size", original.Width, original.Height)
	}

	// Entering interactive mode: exactly one resize-window call, exactly
	// one SIGWINCH.
	if _, err := client.FitWindowToPane(ctx, target, target, 45, 15); err != nil {
		t.Fatalf("fit window to pane (enter): %v", err)
	}
	waitForWinchCount(t, winchLog, 1)

	// Attach a real client at a THIRD size. window-size is "manual" at
	// this point (the fit above), so attaching must NOT move the window
	// or cost a SIGWINCH of its own -- interactive mode's window stays
	// exactly where the fit left it while a client looks at it.
	const clientCols, clientRows = 100, 40
	terminal, cmd := attachThroughPTY(t, socket, target, clientCols, clientRows)
	defer func() { _ = terminal.Close() }()
	waitForSessionAttachedCount(t, client, target, 1)

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, target)
	if err != nil {
		t.Fatalf("read pane size after attach: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size after attach = %dx%d, want it to stay at the interactive 45x15 (window-size=manual must ignore the attaching client)", paneWidth, paneHeight)
	}
	if got := readWinchCount(t, winchLog); got != 1 {
		t.Fatalf("SIGWINCH count after attach alone = %d, want still 1 -- attaching to a manual-sized window must not itself cost a SIGWINCH", got)
	}

	// Exit: RestoreWindowGeometry with this client attached must SKIP the
	// resize-window step and rely on the unset alone.
	if err := client.RestoreWindowGeometry(ctx, target, original); err != nil {
		t.Fatalf("restore window geometry with a client attached: %v", err)
	}

	_, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, target)
	if err != nil {
		t.Fatalf("read pane size after restore: %v", err)
	}
	// One row short of the attached client's raw terminal size: its
	// status line (on by default) is drawn by the client itself and
	// subtracted before "window-size latest" sizes the window to fit --
	// see TestReversedRestoreOrderLeavesWindowPinned's identical note.
	if paneWidth != clientCols || paneHeight != clientRows-1 {
		t.Fatalf("pane size after restore = %dx%d, want %dx%d (the attached client's own %dx%d, minus its one-row status line) -- the unset alone must hand the window straight back to it, not to the pre-entry %dx%d", paneWidth, paneHeight, clientCols, clientRows-1, clientCols, clientRows, original.Width, original.Height)
	}
	// Exactly two SIGWINCH total: one for entering (the fit above), one
	// for the unset's automatic follow of the attached client -- NOT a
	// third from an explicit resize-window call on exit, which
	// RestoreWindowGeometry must not have issued since attached != 0.
	waitForWinchCount(t, winchLog, 2)
	if got := readWinchCount(t, winchLog); got != 2 {
		t.Fatalf("SIGWINCH count after restore = %d, want exactly 2 (one to enter, one for the unset's automatic follow) -- a 3rd would mean exit issued an unnecessary explicit resize-window while a client was attached", got)
	}

	detachAndWait(t, terminal, cmd)
}

// TestFreshClientAtThirdSizeGovernsWindowAfterExit implements PRD II-13's
// positive half: "A fresh client at a third size governs the window after
// exit." Distinct from TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch
// (task 035), which attaches a client BEFORE calling RestoreWindowGeometry
// and proves the unset's automatic follow costs no extra SIGWINCH, this
// test calls RestoreWindowGeometry first, WHILE NOBODY IS ATTACHED (the
// ordinary exit case), and only THEN attaches a fresh client at a THIRD
// size -- distinct from both the original 80x24 and the interactive
// 45x15 -- proving the restore leaves the window in a state where a
// LATER, unrelated client governs it via window-size latest, not merely
// that an already-attached client is followed through the restore call
// itself.
func TestFreshClientAtThirdSizeGovernsWindowAfterExit(t *testing.T) {
	socket := restoreSocket("third-size-positive")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	original, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}

	// Enter interactive mode, then exit it -- with nobody attached, the
	// ordinary detached restore path (task 035/II-9).
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("fit window to pane (enter): %v", err)
	}
	if err := client.RestoreWindowGeometry(ctx, "s0", original); err != nil {
		t.Fatalf("restore window geometry (exit): %v", err)
	}

	// A FRESH client, at a THIRD size distinct from both 80x24 (original)
	// and 45x15 (the interactive preview), attaches only AFTER exit has
	// already completed.
	const thirdClientCols, thirdClientRows = 100, 40
	terminal, cmd := attachThroughPTY(t, socket, "s0", thirdClientCols, thirdClientRows)
	defer func() { _ = terminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after fresh attach: %v", err)
	}
	// One row short of the client's raw terminal size: its status line
	// (on by default) is subtracted before window-size latest sizes the
	// window to fit -- the same note TestReversedRestoreOrderLeavesWindowPinned
	// and TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch
	// already record.
	wantWidth, wantHeight := thirdClientCols, thirdClientRows-1
	if paneWidth != wantWidth || paneHeight != wantHeight {
		t.Fatalf("pane size after a fresh client attached post-exit at %dx%d = %dx%d, want %dx%d (that client's own size, minus its one-row status line) -- the restore must leave window-size unset so a LATER client governs the window, not merely one already attached during the restore call itself", thirdClientCols, thirdClientRows, paneWidth, paneHeight, wantWidth, wantHeight)
	}
	detachAndWait(t, terminal, cmd)
}

// TestSkippingRestoreLeavesFreshClientPinnedAtPreviewSize is PRD II-13's
// MANDATORY negative control: "skipping the restore leaves it pinned,
// both while that client is attached and after it detaches." Without
// this control, TestFreshClientAtThirdSizeGovernsWindowAfterExit alone
// would not prove the restore is what made the fresh client's size take
// effect -- it could equally be true that ANY client always governs the
// window regardless of what exit did. This test skips
// RestoreWindowGeometry entirely (never calls it, unlike
// TestReversedRestoreOrderLeavesWindowPinned, which calls the two restore
// primitives directly but in the wrong order) and shows the window stays
// pinned at the interactive preview size for a fresh client at the same
// THIRD size the positive test uses, both while that client is attached
// AND after it detaches -- pinned is not a transient artifact of the
// moment of attaching.
func TestSkippingRestoreLeavesFreshClientPinnedAtPreviewSize(t *testing.T) {
	socket := restoreSocket("third-size-negative")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	if _, err := client.CaptureWindowGeometry(ctx, "s0"); err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}

	// Enter interactive mode -- and deliberately never exit it. No call
	// to RestoreWindowGeometry anywhere in this test: window-size stays
	// "manual" at whatever FitWindowToPane last set it to, exactly as if
	// exit's restore step had been skipped outright.
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("fit window to pane (enter): %v", err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after fit: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after fit = set=%v value=%q, want set=true value=\"manual\" (test assumption violated, not what this test is proving)", set, value)
	}

	// The SAME fresh client, at the SAME third size the positive test
	// uses, attaches with no restore having ever run.
	const thirdClientCols, thirdClientRows = 100, 40
	terminal, cmd := attachThroughPTY(t, socket, "s0", thirdClientCols, thirdClientRows)
	defer func() { _ = terminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size while the fresh client is attached: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size while a fresh client at %dx%d is attached, with the restore skipped, = %dx%d, want it PINNED at the interactive preview's 45x15 -- window-size=manual must ignore this client entirely", thirdClientCols, thirdClientRows, paneWidth, paneHeight)
	}

	detachAndWait(t, terminal, cmd)

	// AFTER the client detaches, the window must still be pinned at the
	// preview size -- nothing about detaching itself unsets window-size,
	// so "pinned" is not merely a property of the moment of attaching.
	_, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after the fresh client detaches: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size after the fresh client detaches, with the restore skipped, = %dx%d, want it STILL pinned at the interactive preview's 45x15", paneWidth, paneHeight)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after detach: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after detach, with the restore skipped, = set=%v value=%q, want set=true value=\"manual\" -- nothing detaches ever unsets, which is exactly why the restore step is not optional", set, value)
	}
}

// TestGlobalWindowSizeWriteDoesNotRestoreWindowLocalPin implements PRD
// II-10: "Prove `set -g window-size latest` is not a restore." This is
// NOT TestReversedRestoreOrderLeavesWindowPinned's mistake (the two correct
// primitives issued in the wrong order) -- it is a different, plausible
// "simplification" of RestoreWindowGeometry that never calls
// unsetWindowSize at all: since Client.Bootstrap already writes
// `window-size latest` at the server-global scope (task 032), an
// implementer could believe re-asserting that global value on exit is
// enough to "restore" the window. It is not: `resize-window` (task 034's
// FitWindowToPane, used here to enter) always writes `window-size manual`
// into the WINDOW scope, which shadows the global value entirely --
// writing the global option again touches a table nothing reads while the
// window-local override is still in effect. docs/spikes/interactive-preview.md's
// finding 4 names this exact trap in AoE's shipping prior art ("AoE's code
// is accidentally right; its comment is wrong, and an implementer
// following the comment ships the broken version"); this test pins the
// broken version so a later "simplification" along the same lines is
// blocked here, not rediscovered against a live agent.
func TestGlobalWindowSizeWriteDoesNotRestoreWindowLocalPin(t *testing.T) {
	socket := restoreSocket("global-write")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	if _, err := client.CaptureWindowGeometry(ctx, "s0"); err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}

	// Enter interactive mode: window-size becomes "manual" window-locally,
	// as a side effect of resize-window, exactly as every other test in
	// this file establishes.
	if _, err := client.FitWindowToPane(ctx, "s0", "s0", 45, 15); err != nil {
		t.Fatalf("fit window to pane (enter): %v", err)
	}

	// The mistaken "restore": write the GLOBAL window-size option to
	// "latest" -- already its value since Client.Bootstrap, and never the
	// scope FitWindowToPane's resize-window touched. RestoreWindowGeometry
	// itself never issues this call; it is exercised directly here to prove
	// the mistake, the same way TestReversedRestoreOrderLeavesWindowPinned
	// calls the two correct primitives directly to prove a different one.
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")

	// The window-local override is untouched by that write: it shadows the
	// global value regardless of what the global value is re-asserted to.
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after the global write: %v", err)
	} else if !set || value != "manual" {
		t.Fatalf("window-size after writing the GLOBAL option = set=%v value=%q, want set=true value=\"manual\" -- a global write must not touch the window-local override left by resize-window", set, value)
	}

	// A FRESH client attaching now, at a third size distinct from both the
	// original 80x24 and the interactive 45x15, must still be IGNORED: the
	// global write did nothing to restore anything.
	const pinnedClientCols, pinnedClientRows = 100, 40
	pinnedTerminal, pinnedCmd := attachThroughPTY(t, socket, "s0", pinnedClientCols, pinnedClientRows)
	defer func() { _ = pinnedTerminal.Close() }()
	waitForSessionAttachedCount(t, client, "s0", 1)

	_, _, paneWidth, paneHeight, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after the pinned attach: %v", err)
	}
	if paneWidth != 45 || paneHeight != 15 {
		t.Fatalf("pane size after a fresh client at %dx%d attaches following the GLOBAL-only \"restore\" = %dx%d, want it to stay PINNED at the interactive preview's 45x15 -- writing the global option is not a restore", pinnedClientCols, pinnedClientRows, paneWidth, paneHeight)
	}

	// Contrast: with that same client still attached, the CORRECT unset
	// (the scope resize-window actually shadowed) hands the window straight
	// to it, proving the failure above is about scope, not about the
	// client or the value "latest" itself.
	if err := client.unsetWindowSize(ctx, "s0"); err != nil {
		t.Fatalf("unset window-size (the correct scope): %v", err)
	}
	_, _, paneWidth, paneHeight, err = client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read pane size after the correct-scope unset: %v", err)
	}
	wantWidth, wantHeight := pinnedClientCols, pinnedClientRows-1 // status line, per this file's other tests
	if paneWidth != wantWidth || paneHeight != wantHeight {
		t.Fatalf("pane size after unsetting the WINDOW-local window-size while the same client stays attached = %dx%d, want %dx%d (that client's own size, minus its one-row status line) -- the window-local scope, not the global value, is what was pinning it", paneWidth, paneHeight, wantWidth, wantHeight)
	}
	detachAndWait(t, pinnedTerminal, pinnedCmd)
}

// attachThroughPTY attaches a real tmux client DIRECTLY to target (the raw
// tmux session name, not a deck slug -- these tests attach to bare
// sessions created outside Client.Create's "deck_"-prefixed naming
// convention, so going through Client.Attach/sessionName would not find
// them) on socket, at the given terminal size, through a real pty. This is
// the same technique TestAttachThroughPTY (tmux_test.go) uses for
// Client.Attach itself, minus the self-reexec: these tests need an
// attached client to observe from the OUTSIDE (session_attached, window
// size), not to exercise Client.Attach's own argument handling.
func attachThroughPTY(t *testing.T, socket, target string, cols, rows uint16) (*os.File, *exec.Cmd) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "tmux", "-L", socket, "attach-session", "-t", target)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		t.Fatalf("attach %q through pty at %dx%d: %v", target, cols, rows, err)
	}
	// Drain the attached client's own output so it never blocks on a full
	// pty buffer; none of these tests assert on screen content, only on
	// tmux's own geometry/attachment state.
	go func() { _, _ = io.CopyBuffer(io.Discard, terminal, make([]byte, 4096)) }()
	return terminal, cmd
}

// detachAndWait sends the tmux detach chord (prefix + d) through terminal
// and waits for cmd to exit cleanly, the same shape TestAttachThroughPTY
// already asserts.
func detachAndWait(t *testing.T, terminal *os.File, cmd *exec.Cmd) {
	t.Helper()
	if _, err := terminal.Write([]byte("\x02d")); err != nil {
		t.Fatalf("send detach chord: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("attach did not return cleanly after detach: %v", err)
	}
}

// waitForSessionAttachedCount polls target's #{session_attached} until it
// reads want or a deadline expires, the same shape
// fake_agent_size_test.go's waitForSigwinchCount uses in the features
// package -- attaching a client through a pty is asynchronous from this
// goroutine's point of view.
func waitForSessionAttachedCount(t *testing.T, client Client, target string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last int
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = client.SessionAttachedCount(context.Background(), target)
		if lastErr == nil && last == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session_attached on %q did not reach %d within the deadline (last=%d err=%v)", target, want, last, lastErr)
}

// readWinchCount reads the WINCH-trap counter file written by
// TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch's
// pane script: one 'x' byte per SIGWINCH received. A missing file (no
// SIGWINCH observed yet) reads as zero, the same convention
// fake_agent_size_test.go's readSigwinchCount uses for its own counter
// file.
func readWinchCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read winch count %q: %v", path, err)
	}
	return len(data)
}

// waitForWinchCount polls path until readWinchCount reports want or a
// deadline expires.
func waitForWinchCount(t *testing.T, path string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		last = readWinchCount(t, path)
		if last == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("winch count at %q did not reach %d within the deadline (last=%d)", path, want, last)
}
