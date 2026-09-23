package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestStolenFromExitInteractiveTouchesNeitherOptionNorGeometry proves
// task 112's whole point: once a claim has been stolen (an `F` steal by a
// second model against the same window), the STOLEN-FROM model's own
// exitInteractive -- which still believes it holds the claim, having no
// way to have observed the steal itself -- issues zero resize-window
// calls and leaves BOTH window options (OwnershipOption and
// IsizeGeometryOption) exactly as the winner (the stealer) set them.
// Without task 112's still-mine gate, exitInteractive's teardown would
// unconditionally RestoreWindowGeometry to the stolen-from model's own
// pre-entry geometry and clobber the winner's live claim -- exactly the
// hazard R100's teardown gating exists to prevent.
func TestStolenFromExitInteractiveTouchesNeitherOptionNorGeometry(t *testing.T) {
	socket := selectionTestSocket("stolengate")
	session := "deck_stolengate"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("stolengate")
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
		m.sessions = []store.Session{{ID: "sess-stolengate-1", Name: "stolengate", Slug: "stolengate", Status: "waiting"}}
		m.selected = rowCursor(0)
		return m
	}

	// First entry (`\u21b5`, force=false) at one preview size.
	next1, _ := newTestModel(100, 30).enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry did not enter interactive mode: attachError=%q", got1.attachError)
	}

	// Second entry (`F`, force=true) at a DIFFERENT preview size, so the
	// window ends up fitted to a size that does not coincide with got1's
	// own pre-entry geometry -- if got1's exitInteractive wrongly restored
	// its own geometry, the window's post-exit size would visibly differ
	// from what the winner (got2) actually left it at.
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

	// The stolen-from model's own exit: it never observed the steal, so
	// it still believes its claim is live and calls exitInteractive
	// exactly as it would if nothing had happened.
	got1.exitInteractive()

	ownershipAfter, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s vanished after the stolen-from exit", tmux.OwnershipOption)
	}
	if ownershipAfter != ownershipBefore {
		t.Fatalf("%s changed across the stolen-from exit: before=%q after=%q, want untouched (still the winner's own claim)", tmux.OwnershipOption, ownershipBefore, ownershipAfter)
	}
	isizeAfter, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s vanished after the stolen-from exit", tmux.IsizeGeometryOption)
	}
	if isizeAfter != isizeBefore {
		t.Fatalf("%s changed across the stolen-from exit: before=%q after=%q, want untouched", tmux.IsizeGeometryOption, isizeBefore, isizeAfter)
	}
	geometryAfter, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the stolen-from exit: %v", err)
	}
	if geometryAfter != geometryBefore {
		t.Fatalf("window geometry changed across the stolen-from exit (proves an unwanted resize-window/unset ran): before=%+v after=%+v", geometryBefore, geometryAfter)
	}

	// Clean up through the surviving (winning) claim only.
	got2.exitInteractive()
}
