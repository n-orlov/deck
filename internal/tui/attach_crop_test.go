package tui

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestPassiveFitLeavesNoWindowSizePinBehindIt is the operator's report at
// the level deck causes it: navigating the list must not leave the selected
// session's window pinned, because the pin is what crops the next `a`.
//
// It runs previewFit's real closure against a real tmux server (the same
// shape TestPreviewFitStandsDownNonLatchingUnderForeignLiveClaim uses) and
// asserts BOTH halves of what passive fit owes: the window really is fitted
// to the panel's own content box, and the window-local `window-size` is
// unset afterwards, so §3.2's global `latest` governs the next client to
// attach. Asserting only the second would pass on a fit that stopped
// fitting.
func TestPassiveFitLeavesNoWindowSizePinBehindIt(t *testing.T) {
	socket := selectionTestSocket("previewfitunpin")
	const slug = "previewfitunpin"
	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)

	windowTarget, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	ctx := context.Background()
	client := tmux.Client{Socket: socket}

	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-previewfitunpin-1", Slug: slug, Name: slug, Status: "running"}}
	m.selected = 0
	wantWidth, wantHeight := m.previewContentSize()
	if wantHeight < interactiveMinInnerRows {
		t.Fatalf("test setup: preview content height %d below the %d-row floor", wantHeight, interactiveMinInnerRows)
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick returned %d commands, want 2 (the reschedule plus the passive fit)", len(cmds))
	}
	done, ok := cmds[1]().(previewFitDone)
	if !ok {
		t.Fatalf("the fit command produced %T, want previewFitDone", cmds[1]())
	}
	if done.noLivePane || done.foreignLiveClaim {
		t.Fatalf("previewFitDone = %+v, want a real fit (this window has a live pane and no claim on it)", done)
	}

	geometry, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the passive fit: %v", err)
	}
	if geometry.Width != wantWidth || geometry.Height != wantHeight {
		t.Fatalf("window after the passive fit = %dx%d, want the panel's own content box %dx%d", geometry.Width, geometry.Height, wantWidth, wantHeight)
	}
	if geometry.WindowSizeSet {
		t.Fatalf("window-local window-size = %q after a passive fit, want unset -- this pin is exactly what crops the next `a` (and any bare tmux attach), for as long as it survives", geometry.WindowSizeValue)
	}
}

// TestFullAttachReleasesAPreviewPinButNeverALiveOwnersPin covers `a`'s own
// half of the operator's rule: "`a` must not be cropped by deck's own
// preview in all cases. Only other tmux sessions (deck or tmux direct) can
// cause crop."
//
// Both directions are asserted against a real tmux window, because a
// release that also fired under a live owner would be strictly worse than
// the crop it fixes -- it would resize the window under another deck's
// interactive grid the moment this client attached.
func TestFullAttachReleasesAPreviewPinButNeverALiveOwnersPin(t *testing.T) {
	for _, tc := range []struct {
		name       string
		claimed    bool
		wantPinned bool
	}{
		{name: "a stale pin with nobody holding the window is released", claimed: false, wantPinned: false},
		{name: "a live owner's pin is left exactly as it is", claimed: true, wantPinned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socket := selectionTestSocket("attachunpin")
			const slug = "attachunpin"
			newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
			windowTarget, err := tmux.SessionName(slug)
			if err != nil {
				t.Fatalf("SessionName: %v", err)
			}
			ctx := context.Background()
			client := tmux.Client{Socket: socket}

			// The pin a fit leaves behind -- here written directly, standing
			// in for the process that wrote it: an older deck build sharing
			// this socket, a SIGKILLed interactive session, or (in the
			// claimed case) the live holder below.
			if err := client.PinWindowSize(ctx, windowTarget); err != nil {
				t.Fatalf("pin window-size for the setup: %v", err)
			}
			if tc.claimed {
				// A live foreign claim, exactly as
				// TestPreviewFitStandsDownNonLatchingUnderForeignLiveClaim
				// builds one: this test binary is the claiming pid, and it
				// is trivially alive to a liveness check.
				ownership, owned, err := client.ClaimWindowOwnership(ctx, windowTarget)
				if err != nil || !owned {
					t.Fatalf("claim %s for the live-owner case: owned=%v err=%v", windowTarget, owned, err)
				}
				t.Cleanup(func() { _ = ownership.Release(context.Background()) })
			}

			m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
			m.width, m.height = 100, 30
			m.tmuxClient = client
			m.sessions = []store.Session{{ID: "sess-attachunpin-1", Slug: slug, Name: slug, Status: "running"}}
			m.selected = 0
			var attached int
			m.attach = func(context.Context, string) (*exec.Cmd, error) {
				attached++
				// Never run: Update hands this to tea.ExecProcess, which the
				// test does not execute. The attach's own behaviour is not
				// what is under test here; the window it hands over is.
				return exec.Command("true"), nil
			}

			// Through the real key path, not attachSelected directly: what the
			// operator pressed is `a`.
			m = pressKey(t, m, "a")
			if attached != 1 {
				t.Fatalf("`a` built %d attach commands, want exactly 1 -- with none, the pin assertions below prove nothing", attached)
			}
			if m.attachError != "" {
				t.Fatalf("`a` refused the attach: %q", m.attachError)
			}

			geometry, err := client.CaptureWindowGeometry(ctx, windowTarget)
			if err != nil {
				t.Fatalf("CaptureWindowGeometry after `a`: %v", err)
			}
			if tc.wantPinned {
				if !geometry.WindowSizeSet || geometry.WindowSizeValue != "manual" {
					t.Fatalf("window-local window-size after `a` = set=%v value=%q, want it left at \"manual\" -- a live owner is holding that size for a grid it is drawing, and only that owner may release it", geometry.WindowSizeSet, geometry.WindowSizeValue)
				}
				return
			}
			if geometry.WindowSizeSet {
				t.Fatalf("window-local window-size after `a` = %q, want unset -- the attaching client must express its own size, not inherit whatever preview panel last fitted this window", geometry.WindowSizeValue)
			}
		})
	}
}

// TestAttachFinishedRelicensesOneFitForTheAttachedRow closes the loop the
// release opens: the full attach resizes the window to the departing
// client's own terminal and tmux leaves it there on detach, so the row the
// user lands back on is the one row passive fit's per-session coalescing
// would never re-fit. Coming back from `a` must relicense exactly one fit
// -- the same carve-out SPEC §11 already makes for a relaunch.
func TestAttachFinishedRelicensesOneFitForTheAttachedRow(t *testing.T) {
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "deck-attachfinished-unused"}
	m.sessions = []store.Session{{ID: "s1", Slug: "s1", Name: "s1", Status: "running"}}
	m.selected = 0
	m.previewFitSessionID = "s1"

	// Before: the latch suppresses the fit, which is the whole point of it.
	if cmd := m.previewFit(); cmd != nil {
		t.Fatal("previewFit issued a fit for an already-latched selection; the coalescing guard is what this test is about")
	}

	updated, _ := m.Update(attachFinished{})
	m = updated.(Model)
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q after attachFinished, want cleared -- otherwise the row the user just attached to keeps the attach's own terminal size in the preview panel until they select away and back", m.previewFitSessionID)
	}
	if cmd := m.previewFit(); cmd == nil {
		t.Fatal("previewFit issued nothing after attachFinished cleared the latch, want one fit for the still-selected row")
	}
	if m.previewFitInFlight != "s1" {
		t.Fatalf("previewFitInFlight = %q, want %q -- the relicensed fit must still take the in-flight marker", m.previewFitInFlight, "s1")
	}
}

// TestPassiveFitStandsDownForAnAttachedClientAndReleasesItsPin is the
// preview side of the same rule `a` keeps: while a real client is attached
// to a session, deck's preview must not pull that session's window into
// deck's own panel box.
//
// tmux's `window-size latest` means the attached client owns the size, so
// a fit here could only hold it by pinning -- which is the crop -- and
// even a fit that then unpinned would bounce a terminal somebody is
// looking at through two resizes. So the closure skips the resize and
// issues the unpin alone, which is the half that matters: it releases a
// pin an EARLIER fit left behind, so the attach stops being cropped.
//
// The setup is that exact crop: a client attaches at 80x24, then a plain
// FitWindowToPane (what passive fit used to be) drags the window down to
// the panel's box and pins it there. What the next passive fit does is
// then asserted three ways -- zero resize-window commands on the wire, one
// window-size unset on the wire, and the window sitting at the client's
// own size afterwards -- because the final size alone cannot tell a
// stand-down from a re-fit that the unpin undid.
func TestPassiveFitStandsDownForAnAttachedClientAndReleasesItsPin(t *testing.T) {
	socket := selectionTestSocket("fitattached")
	const slug = "fitattached"
	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
	windowTarget, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	ctx := context.Background()
	client := tmux.Client{Socket: socket}

	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.width, m.height = 100, 30
	binary, wireLog := newTmuxWireLogger(t)
	m.tmuxClient = tmux.Client{Socket: socket, Binary: binary}
	m.sessions = []store.Session{{ID: "sess-fitattached-1", Slug: slug, Name: slug, Status: "running"}}
	m.selected = 0
	panelWidth, panelHeight := m.previewContentSize()
	if panelHeight < interactiveMinInnerRows {
		t.Fatalf("test setup: preview content height %d below the %d-row floor", panelHeight, interactiveMinInnerRows)
	}

	// A real client, and then the crop: the window fitted to deck's panel
	// and pinned there, which is what a plain FitWindowToPane leaves.
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)
	const attachedCols, attachedRows = 80, 24
	pane, ok, err := client.PreviewPane(ctx, slug)
	if err != nil || !ok {
		t.Fatalf("PreviewPane for the setup: ok=%v err=%v", ok, err)
	}
	if _, err := client.FitWindowToPane(ctx, windowTarget, pane.ID, panelWidth, panelHeight); err != nil {
		t.Fatalf("fit the window to the panel for the setup: %v", err)
	}
	cropped, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the setup fit: %v", err)
	}
	if cropped.Width != panelWidth || cropped.Height != panelHeight || !cropped.WindowSizeSet {
		t.Fatalf("test setup: want the window cropped to %dx%d and pinned, got %dx%d pinned=%v", panelWidth, panelHeight, cropped.Width, cropped.Height, cropped.WindowSizeSet)
	}
	if cropped.Width == attachedCols && cropped.Height == attachedRows-1 {
		t.Fatalf("test setup: the panel box %dx%d coincides with the attached client's own size, so a stand-down would be invisible", panelWidth, panelHeight)
	}

	// Everything the model puts on the wire from here is the fit attempt.
	truncateWireLog(t, wireLog)
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick returned %d commands, want 2 (the reschedule plus the passive fit)", len(cmds))
	}
	done, ok := cmds[1]().(previewFitDone)
	if !ok {
		t.Fatalf("the fit command produced %T, want previewFitDone", cmds[1]())
	}
	if !done.clientAttached {
		t.Fatalf("previewFitDone = %+v, want clientAttached -- a real client is attached to this session", done)
	}

	if resizes := countWireCommands(t, wireLog, "resize-window"); resizes != 0 {
		t.Fatalf("passive fit issued %d resize-window commands with a client attached, want 0 -- resizing here reflows a terminal somebody is looking at, for a size the client takes straight back", resizes)
	}
	if unsets := countWireOptionUnsets(t, wireLog, "window-size"); unsets != 1 {
		t.Fatalf("passive fit issued %d window-size unsets, want exactly 1 -- releasing an earlier fit's pin is the whole of what it may do while a client is attached", unsets)
	}

	geometry, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the stand-down: %v", err)
	}
	if geometry.WindowSizeSet {
		t.Fatalf("window-local window-size = %q after the stand-down, want unset", geometry.WindowSizeValue)
	}
	if geometry.Width != attachedCols || geometry.Height != attachedRows-1 {
		t.Fatalf("window after the stand-down = %dx%d, want the attached client's own %dx%d (its %dx%d terminal minus tmux's status line) -- the unpin must hand the window back to it", geometry.Width, geometry.Height, attachedCols, attachedRows-1, attachedCols, attachedRows)
	}

	// Non-latching, exactly like the foreign-live-claim stand-down: the
	// attach is transient, and the row must regain its fit the moment that
	// client detaches, without the user selecting away and back.
	settled, _ := m.Update(done)
	if got := settled.(Model).previewFitSessionID; got != "" {
		t.Fatalf("previewFitSessionID = %q after an attached-client stand-down, want it left unlatched", got)
	}
}
