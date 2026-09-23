package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// countWireOptionUnsets counts the logged tmux invocations that unset one
// specific option in the window scope -- the exact wire shape every
// teardown step this file cares about produces, and the only way to tell
// them apart from each other, since all three are `set-option`:
//
//	RestoreWindowGeometry's step 2  set-option -w -u -t <target> window-size
//	ClearIsizeGeometry              set-option -w -u -t <target> @deck_isize_geometry
//	WindowOwnership.Release         set-option -w -u -t <target> @deck_isize_owner
//
// countWireCommands (force_indistinguishable_test.go) only looks at the
// first token after the shim's `-L <socket>` prefix, so it cannot
// distinguish those three; this counts by the `-u` flag plus the option
// name in final position instead, and deliberately does NOT pin the target
// token (the entry path chooses its own window target form).
//
// It is the observable that answers the one question the resulting tmux
// STATE cannot: `Release` self-gates (it re-reads the option and returns
// nil when the value is not its own claim), so "the option still holds the
// winner's claim" is equally consistent with Release having been called and
// with Release never being reached. "Zero ownership-option unsets on this
// client's wire" is not: it fails the moment the release step actually
// runs.
func countWireOptionUnsets(t *testing.T, logPath, option string) int {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux wire log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// Every invocation the shim sees is `-L <socket> <command> ...`.
		if len(fields) < 4 || fields[0] != "-L" || fields[2] != "set-option" {
			continue
		}
		if fields[len(fields)-1] != option {
			continue
		}
		for _, field := range fields[3:] {
			if field == "-u" {
				count++
				break
			}
		}
	}
	return count
}

// countWireOptionReads counts the logged tmux invocations that READ one
// specific window-scoped option (`show-options -wv -t <target> <option>`),
// as opposed to countWireOptionUnsets above which counts the option's
// UNSET. Every read of OwnershipOption on the wire -- whether from
// claimStillMine's own probe or from WindowOwnership.Release's internal
// self-gating re-read (Release always re-reads before deciding whether to
// unset, tmux/ownership.go's own Release) -- takes this exact shape, so
// counting reads is what makes "Release was never reached" and "Release
// ran and self-gated into a no-op" distinguishable on the wire: both
// leave the unset count at zero, but only the first leaves the read count
// at exactly one (the still-mine probe alone) rather than two.
func countWireOptionReads(t *testing.T, logPath, option string) int {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux wire log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// Every invocation the shim sees is `-L <socket> <command> ...`.
		if len(fields) < 4 || fields[0] != "-L" || fields[2] != "show-options" {
			continue
		}
		if fields[len(fields)-1] == option {
			count++
		}
	}
	return count
}

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
//
// The claim is made on the WIRE, not merely on the resulting tmux state:
// the stolen-from model's own tmux.Client runs through its private
// wire-logging shim (newTmuxWireLogger), the log is truncated immediately
// before raiseLostAttach, and the span afterwards must contain zero
// `resize-window` invocations and zero window-option unsets of any kind.
// A redundant same-size resize, or a Release call that self-gated into a
// no-op, both fail these counts while leaving the asserted state below
// completely unchanged.
func TestRaiseLostAttachOnStolenClaimTouchesNothing(t *testing.T) {
	socket := selectionTestSocket("teardownstolen")
	session := "deck_teardownstolen"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	// The stolen-from model gets its own logging shim so the winner's own
	// (legitimate) claim/fit traffic can never be counted against it.
	loserBinary, loserLog := newTmuxWireLogger(t)
	loserClient := tmux.Client{Socket: socket, Binary: loserBinary}

	windowTarget, err := tmux.SessionName("teardownstolen")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	newTestModel := func(width, height int, modelClient tmux.Client) Model {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = width, height
		if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
		}
		m.tmuxClient = modelClient
		m.sessions = []store.Session{{ID: "sess-teardownstolen-1", Name: "teardownstolen", Slug: "teardownstolen", Status: "waiting"}}
		m.selected = rowCursor(0)
		return m
	}

	// First entry: the model whose claim is about to be stolen.
	next1, _ := newTestModel(100, 30, loserClient).enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry did not enter interactive mode: attachError=%q", got1.attachError)
	}

	// The steal: a second entry, force=true, at a DIFFERENT preview size
	// so the winner's own fitted geometry visibly differs from got1's
	// pre-entry geometry -- a wrongful restore back to got1's own
	// pre-entry size would otherwise be invisible.
	next2, _ := newTestModel(120, 40, client).enterInteractiveBody(true)
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
	// previewTick's fast path would drive it. Everything this model's own
	// client puts on the wire from here on is exactly the teardown span.
	truncateWireLog(t, loserLog)
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

	// Zero resize-window calls, counted on the wire: not "the size ended
	// up the same", but "the command was never issued", so a redundant
	// same-size resize fails here too.
	if resizes := countWireCommands(t, loserLog, "resize-window"); resizes != 0 {
		t.Fatalf("the stolen-from teardown issued %d resize-window commands, want 0 (the still-mine gate must skip RestoreWindowGeometry entirely)", resizes)
	}
	// Releases nothing, counted on the wire: WindowOwnership.Release
	// self-gates, so only the absence of its own unset from this client's
	// wire distinguishes "never called" from "called and declined".
	if unsets := countWireOptionUnsets(t, loserLog, tmux.OwnershipOption); unsets != 0 {
		t.Fatalf("the stolen-from teardown issued %d %s unsets, want 0 (Release must never be reached)", unsets, tmux.OwnershipOption)
	}
	// Task 136: count READS of the ownership option too, not only its
	// unset. WindowOwnership.Release self-gates by re-reading the option
	// before deciding whether to unset it, so a spurious Release call
	// that lands here and correctly declines to unset would still cost a
	// SECOND show-options read of OwnershipOption -- one that an
	// unset-only count is blind to but a read count catches. Exactly one
	// read is claimStillMine's own probe (the still-mine gate above);
	// zero unsets confirms it declined; a second read would mean Release
	// ran anyway.
	//
	// Verified load-bearing by hand for this task: temporarily adding
	// `_ = ownership.Release(ctx)` to teardownInteractiveClaim's
	// `if !stillMine { ... }` branch (internal/tui/interactive.go, right
	// before its `return`) raises this count to 2 and fails this
	// assertion, while leaving every unset-count assertion above at 0
	// (Release's own self-gate declines to unset, exactly as documented).
	// The extra call was reverted immediately after; it must never be
	// left in the tree.
	if reads := countWireOptionReads(t, loserLog, tmux.OwnershipOption); reads != 1 {
		t.Fatalf("the stolen-from teardown issued %d %s show-options reads, want exactly 1 (claimStillMine's own probe; a second read would mean Release ran and merely self-gated into a no-op)", reads, tmux.OwnershipOption)
	}
	if unsets := countWireOptionUnsets(t, loserLog, tmux.IsizeGeometryOption); unsets != 0 {
		t.Fatalf("the stolen-from teardown issued %d %s unsets, want 0 (ClearIsizeGeometry must never be reached)", unsets, tmux.IsizeGeometryOption)
	}
	if unsets := countWireOptionUnsets(t, loserLog, "window-size"); unsets != 0 {
		t.Fatalf("the stolen-from teardown issued %d window-size unsets, want 0 (RestoreWindowGeometry must never be reached)", unsets)
	}
	// Nothing else mutated a window option either: the whole post-probe
	// body is skipped, not merely its three named steps.
	if sets := countWireCommands(t, loserLog, "set-option"); sets != 0 {
		t.Fatalf("the stolen-from teardown issued %d set-option commands, want 0 (nothing may be written after the still-mine probe fails)", sets)
	}

	// The resulting state agrees: the window geometry is byte-identical to
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
//
// As in the stolen-claim test above, each of those four steps is asserted
// as a WIRE count over the teardown span (the model's client runs through
// newTmuxWireLogger's shim, truncated immediately before raiseLostAttach):
// zero `resize-window`, exactly one `window-size` unset, exactly one
// @deck_isize_geometry unset, exactly one @deck_isize_owner unset. The
// final 80x23 state alone would not distinguish the attached gate skipping
// the explicit resize from an explicit resize followed by the unset --
// only the zero resize-window count does.
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
	binary, wireLog := newTmuxWireLogger(t)
	m.tmuxClient = tmux.Client{Socket: socket, Binary: binary}
	m.sessions = []store.Session{{ID: "sess-teardownattach-1", Name: "teardownattach", Slug: "teardownattach", Status: "waiting"}}
	m.selected = rowCursor(0)

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

	// Everything this model's client puts on the wire from here on is
	// exactly the teardown span.
	truncateWireLog(t, wireLog)
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

	// Attached-gated restore, proved on the wire: zero resize-window
	// commands (the attached client is still there, so
	// RestoreWindowGeometry skips step 1) and exactly one window-size
	// unset (step 2, unconditional).
	if resizes := countWireCommands(t, wireLog, "resize-window"); resizes != 0 {
		t.Fatalf("the client-attached teardown issued %d resize-window commands, want 0 (RestoreWindowGeometry's attached gate must skip the explicit resize)", resizes)
	}
	if unsets := countWireOptionUnsets(t, wireLog, "window-size"); unsets != 1 {
		t.Fatalf("the client-attached teardown issued %d window-size unsets, want exactly 1 (RestoreWindowGeometry's unconditional step 2)", unsets)
	}
	if unsets := countWireOptionUnsets(t, wireLog, tmux.IsizeGeometryOption); unsets != 1 {
		t.Fatalf("the client-attached teardown issued %d %s unsets, want exactly 1 (ClearIsizeGeometry)", unsets, tmux.IsizeGeometryOption)
	}
	if unsets := countWireOptionUnsets(t, wireLog, tmux.OwnershipOption); unsets != 1 {
		t.Fatalf("the client-attached teardown issued %d %s unsets, want exactly 1 (WindowOwnership.Release)", unsets, tmux.OwnershipOption)
	}

	// The resulting state agrees: the explicit resize-window step was
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
