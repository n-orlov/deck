package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestRaiseLostAttachOnStolenClaimTouchesNothing proves task 119's first
// half: raiseLostAttach's own exitInteractive call, run from the
// STOLEN-FROM model (the F-steal flavour, task 118's fast path), goes
// through exactly teardown_still_mine_gate_test.go's own still-mine gate
// (task 112) -- claimStillMine reads ClaimForeignLive once the steal has
// landed, so teardownInteractiveClaim's whole post-probe body is skipped:
// zero resize-window calls, @deck_isize_geometry and @deck_isize_owner
// left byte-exact as the winner (the stealer) set them, and Release never
// called. This is the SAME teardown path previewTick's fast path drives
// (interactive_displacement_test.go's TestPreviewTickRaisesLostAttachOnStolenClaim
// proves the Model-level routing into raiseLostAttach; this proves what
// raiseLostAttach's own teardown does to tmux state once it gets there).
func TestRaiseLostAttachOnStolenClaimTouchesNothing(t *testing.T) {
	socket := selectionTestSocket("teardownstolen")
	session := "deck_teardownstolen"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("teardownstolen")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	newTestModel := func(width, height int) Model {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = width, height
		if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
		}
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-teardownstolen-1", Name: "teardownstolen", Slug: "teardownstolen", Status: "waiting"}}
		m.selected = 0
		return m
	}

	// First entry: the model whose claim is about to be stolen.
	next1, _ := newTestModel(100, 30).enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry did not enter interactive mode: attachError=%q", got1.attachError)
	}

	// The steal: a second entry, force=true, at a DIFFERENT preview size
	// so the winner's own fitted geometry visibly differs from got1's
	// pre-entry geometry -- a wrongful restore back to got1's own
	// pre-entry size would otherwise be invisible.
	next2, _ := newTestModel(120, 40).enterInteractiveBody(true)
	got2 := next2.(Model)
	if got2.attachError != "" {
		t.Fatalf("steal refused: %q", got2.attachError)
	}
	if !got2.interactive {
		t.Fatalf("steal did not enter interactive mode")
	}

	ctx := context.Background()

	ownershipBefore, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s unset after the steal", tmux.OwnershipOption)
	}
	isizeBefore, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s unset after the steal", tmux.IsizeGeometryOption)
	}
	geometryBefore, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the steal: %v", err)
	}
	if geometryBefore == got1.interactiveGeometry {
		t.Fatalf("test assumption violated: the two models' geometries coincide (%+v), so a wrongful restore would be invisible", geometryBefore)
	}

	// The stolen-from model raises the lost-attach dialog exactly as
	// previewTick's fast path would drive it.
	m, _ := got1.raiseLostAttach("teardownstolen")
	if m.interactive {
		t.Fatalf("raiseLostAttach did not leave interactive mode")
	}
	if !m.lostAttach {
		t.Fatalf("raiseLostAttach did not raise the lost-attach dialog")
	}
	if m.lostAttachSession != "teardownstolen" {
		t.Fatalf("lostAttachSession = %q, want %q", m.lostAttachSession, "teardownstolen")
	}

	// Zero resize-window calls: the window geometry is byte-identical to
	// what the winner already left it at.
	geometryAfter, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after raiseLostAttach: %v", err)
	}
	if geometryAfter != geometryBefore {
		t.Fatalf("window geometry changed across the stolen-from teardown (proves an unwanted resize-window/unset ran): before=%+v after=%+v", geometryBefore, geometryAfter)
	}

	// Both options left exactly as the winner set them: never touched.
	ownershipAfter, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s vanished across the stolen-from teardown, want untouched (proves Release ran)", tmux.OwnershipOption)
	}
	if ownershipAfter != ownershipBefore {
		t.Fatalf("%s changed across the stolen-from teardown: before=%q after=%q, want untouched", tmux.OwnershipOption, ownershipBefore, ownershipAfter)
	}
	isizeAfter, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s vanished across the stolen-from teardown, want untouched", tmux.IsizeGeometryOption)
	}
	if isizeAfter != isizeBefore {
		t.Fatalf("%s changed across the stolen-from teardown: before=%q after=%q, want untouched", tmux.IsizeGeometryOption, isizeBefore, isizeAfter)
	}

	// Clean up through the surviving (winning) claim only.
	got2.exitInteractive()
}

// TestRaiseLostAttachOnClientAttachRunsOrdinaryTeardown proves task 119's
// second half: raiseLostAttach's own exitInteractive call, run from a
// model whose claim was never stolen (the client-attached flavour, task
// 118's backstop), runs teardownInteractiveClaim's ordinary full body --
// claimStillMine still reads ClaimStillMine (nothing ever touched the
// ownership option) -- with RestoreWindowGeometry's own attached-gate
// (PRD II-9) skipping the explicit resize-window call because a real
// client is still attached at teardown time: per
// internal/tmux/restore_test.go's own
// TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch,
// window-size's unconditional unset alone hands the window straight to
// that attached client's own size (minus its one-row status line) via
// tmux's "window-size latest" follow, NOT back to the pre-entry geometry
// and NOT staying pinned at the fitted size -- while @deck_isize_geometry
// is cleared and @deck_isize_owner is released.
func TestRaiseLostAttachOnClientAttachRunsOrdinaryTeardown(t *testing.T) {
	socket := selectionTestSocket("teardownattach")
	session := "deck_teardownattach"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("teardownattach")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
	}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-teardownattach-1", Name: "teardownattach", Slug: "teardownattach", Status: "waiting"}}
	m.selected = 0

	ctx := context.Background()

	preEntry, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry before entry: %v", err)
	}

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}

	// A REAL tmux client attaches directly (the client-attached flavour):
	// this never touches ownership or the isize record, but it does make
	// #{session_attached} nonzero for RestoreWindowGeometry's own gate.
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)

	fitted, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after fit+attach: %v", err)
	}
	if !fitted.WindowSizeSet {
		t.Fatalf("test assumption violated: window-size is not set window-locally after FitWindowToPane, got %+v", fitted)
	}
	if fitted.Width == preEntry.Width && fitted.Height == preEntry.Height {
		t.Fatalf("test assumption violated: the fitted geometry %+v coincides with the pre-entry geometry %+v, so a skipped resize-window would be invisible", fitted, preEntry)
	}

	ownershipBefore, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s unset before teardown", tmux.OwnershipOption)
	}
	_ = ownershipBefore
	if _, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); !ok {
		t.Fatalf("%s unset before teardown", tmux.IsizeGeometryOption)
	}

	after, _ := got.raiseLostAttach("teardownattach")
	if after.interactive {
		t.Fatalf("raiseLostAttach did not leave interactive mode")
	}
	if !after.lostAttach {
		t.Fatalf("raiseLostAttach did not raise the lost-attach dialog")
	}
	if after.lostAttachSession != "teardownattach" {
		t.Fatalf("lostAttachSession = %q, want %q", after.lostAttachSession, "teardownattach")
	}

	// Attached-gated restore: the explicit resize-window step was
	// skipped (the attached client is still there, so RestoreWindowGeometry
	// never issues it), but the unconditional window-size unset alone hands
	// the window straight to the still-attached client's own size (the
	// attachForceEnterPTY pty above is 80x24; minus its one-row status line
	// is 80x23) -- neither the fitted size nor the pre-entry size.
	const attachedPTYCols, attachedPTYRows = 80, 24
	postTeardown, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after teardown: %v", err)
	}
	if postTeardown.Width == fitted.Width && postTeardown.Height == fitted.Height {
		t.Fatalf("window dimensions stayed pinned at the fitted size across an attached-client teardown, want them to follow the attached client via the unset: fitted=%+v after=%+v", fitted, postTeardown)
	}
	if postTeardown.Width != attachedPTYCols || postTeardown.Height != attachedPTYRows-1 {
		t.Fatalf("window dimensions after teardown = %dx%d, want %dx%d (the attached client's own %dx%d, minus its one-row status line) -- the unset alone must hand the window straight back to it", postTeardown.Width, postTeardown.Height, attachedPTYCols, attachedPTYRows-1, attachedPTYCols, attachedPTYRows)
	}
	if postTeardown.WindowSizeSet {
		t.Fatalf("window-size is still set window-locally after teardown, want unset: %+v", postTeardown)
	}

	if _, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("%s still present after the client-attached teardown, want released", tmux.OwnershipOption)
	}
	if _, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("%s still present after the client-attached teardown, want cleared", tmux.IsizeGeometryOption)
	}
}
