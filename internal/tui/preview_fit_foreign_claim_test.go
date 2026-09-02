package tui

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestPreviewFitStandsDownNonLatchingUnderForeignLiveClaim pins task 124's
// R102 stand-down (SPEC.md §11.9) end to end against a REAL tmux server,
// not the mocked previewFitDone values preview_fit_overlap_test.go's own
// three tests construct by hand: this one runs the previewFit closure
// itself (client.ProbeWindowOwnership included) so the foreignLiveClaim
// path is proved on the wire, not merely on the struct literal it produces.
//
// Two properties, both required by task 125:
//
//  1. A tick under a foreign live claim on the window delivers a
//     previewFitDone{foreignLiveClaim: true} that (a) issues ZERO
//     resize-window calls on the wire and (b) does NOT latch
//     previewFitSessionID once Update processes it -- unlike a real fit,
//     which always latches (task 035/124's own non-latching contrast).
//  2. Once the claim is released, the VERY NEXT tick for the same
//     session -- previewFitSessionID never having latched -- issues a
//     real fit (exactly one resize-window call) and DOES latch
//     previewFitSessionID, proving the stand-down in property 1 was a
//     fresh-every-attempt read, not a refusal that would have wedged the
//     session for the rest of the model's lifetime.
func TestPreviewFitStandsDownNonLatchingUnderForeignLiveClaim(t *testing.T) {
	socket := selectionTestSocket("previewfitforeign")
	const slug = "previewfitforeign"
	// 80x24: the same session geometry displacement_teardown_test.go's own
	// tests use against a 100x30 model, chosen there (and reused here) so
	// the fitted preview content size provably differs from the window's
	// starting size -- otherwise a skipped/issued resize-window would be
	// indistinguishable from a same-size no-op.
	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)

	windowTarget, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	ctx := context.Background()

	binary, wireLog := newTmuxWireLogger(t)
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.width, m.height = 100, 30
	if !m.computeLayout().PreviewShown {
		t.Fatal("test setup: PreviewShown = false at 100x30, want true")
	}
	if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
		t.Fatalf("test setup: preview content height below the %d-row floor", interactiveMinInnerRows)
	}
	m.tmuxClient = tmux.Client{Socket: socket, Binary: binary}
	m.sessions = []store.Session{{ID: "sess-previewfitforeign-1", Slug: slug, Name: slug, Status: "running"}}
	m.selected = 0

	// A foreign LIVE claim on the very window previewFit is about to
	// probe -- a plain ClaimWindowOwnership by a client that is not the
	// model's own is exactly a live, non-"mine" claim from the target-only
	// Client.ProbeWindowOwnership's point of view (internal/tmux/claim_
	// probe_test.go's own TestClaimStateProbeForeignLiveOnAContestedWindow
	// establishes the same thing this way: the claiming process is THIS
	// test binary, always alive to a liveness check).
	rawClient := tmux.Client{Socket: socket}
	ownership, owned, err := rawClient.ClaimWindowOwnership(ctx, windowTarget)
	if err != nil || !owned {
		t.Fatalf("claim %s for the foreign-live setup: owned=%v err=%v", windowTarget, owned, err)
	}

	// --- Property 1: the tick under the foreign claim. ---
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick under a foreign live claim returned %d commands, want 2 (the reschedule plus one passive-fit attempt)", len(cmds))
	}
	if m.previewFitInFlight != "sess-previewfitforeign-1" {
		t.Fatalf("previewFitInFlight = %q after scheduling, want the session's own id", m.previewFitInFlight)
	}

	fitMsg := cmds[1]()
	done, ok := fitMsg.(previewFitDone)
	if !ok {
		t.Fatalf("the fit command produced %T, want previewFitDone", fitMsg)
	}
	if done.sessionID != "sess-previewfitforeign-1" {
		t.Fatalf("previewFitDone.sessionID = %q, want %q", done.sessionID, "sess-previewfitforeign-1")
	}
	if !done.foreignLiveClaim {
		t.Fatalf("previewFitDone.foreignLiveClaim = false under a live foreign claim, want true")
	}
	if done.noLivePane {
		t.Fatalf("previewFitDone.noLivePane = true, want false -- the pane is live, only the window is foreign-claimed")
	}
	if resizes := countWireCommands(t, wireLog, "resize-window"); resizes != 0 {
		t.Fatalf("the foreign-claimed tick issued %d resize-window commands, want 0 -- the stand-down must skip FitWindowToPane entirely", resizes)
	}

	updated, _ = m.Update(done)
	m = updated.(Model)
	if m.previewFitInFlight != "" {
		t.Fatalf("previewFitInFlight = %q after previewFitDone landed, want cleared", m.previewFitInFlight)
	}
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q after a foreignLiveClaim previewFitDone, want empty -- a stand-down must NOT latch, or the session never becomes eligible again once the claim clears", m.previewFitSessionID)
	}

	// --- Property 2: released, the very next tick fits for real. ---
	if err := ownership.Release(ctx); err != nil {
		t.Fatalf("release the foreign-live claim: %v", err)
	}
	truncateWireLog(t, wireLog)

	updated, cmd = m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds = previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick after the claim was released returned %d commands, want 2 (previewFitSessionID never latched, so the session is still eligible)", len(cmds))
	}

	fitMsg = cmds[1]()
	done2, ok := fitMsg.(previewFitDone)
	if !ok {
		t.Fatalf("the second fit command produced %T, want previewFitDone", fitMsg)
	}
	if done2.sessionID != "sess-previewfitforeign-1" {
		t.Fatalf("second previewFitDone.sessionID = %q, want %q", done2.sessionID, "sess-previewfitforeign-1")
	}
	if done2.foreignLiveClaim {
		t.Fatalf("second previewFitDone.foreignLiveClaim = true after the claim was released, want false -- the claim is read fresh on every attempt")
	}
	if done2.noLivePane {
		t.Fatalf("second previewFitDone.noLivePane = true, want false")
	}
	if resizes := countWireCommands(t, wireLog, "resize-window"); resizes != 1 {
		t.Fatalf("the post-release tick issued %d resize-window commands, want exactly 1 -- the session now fits for real", resizes)
	}

	updated, _ = m.Update(done2)
	m = updated.(Model)
	if m.previewFitSessionID != "sess-previewfitforeign-1" {
		t.Fatalf("previewFitSessionID = %q after a real fit's previewFitDone, want the session's own id -- a real fit DOES latch, unlike the stand-down above", m.previewFitSessionID)
	}
}
