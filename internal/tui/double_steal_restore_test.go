package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestTwoSequentialStealsRestorePreEntryGeometry proves task 114's whole
// point: TWO stolen claims in a row on the same window -- entry, steal,
// steal -- still end at a byte-exact restore of the pre-first-entry
// geometry once the LAST (surviving, never-stolen-from) holder exits
// legitimately. Every earlier claim was stolen from and so, per task 112,
// its own exitInteractive would be a no-op even if called (it is not
// called here, mirroring teardown_still_mine_gate_test.go): the ONLY
// teardown that ever runs is the final winner's, and R100's contract is
// that its own restore target is the ORIGINAL geometry adopted at the
// very first claim (task 111's ResolveIsizeGeometry), not either
// intermediate model's own fitted size.
func TestTwoSequentialStealsRestorePreEntryGeometry(t *testing.T) {
	socket := selectionTestSocket("doublesteal")
	session := "deck_doublesteal"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("doublesteal")
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
		m.sessions = []store.Session{{ID: "sess-doublesteal-1", Name: "doublesteal", Slug: "doublesteal", Status: "waiting"}}
		m.selected = 0
		return m
	}

	ctx := context.Background()

	// Capture the window's pre-first-entry geometry -- width, height and
	// the window-size read, INCLUDING its unset shape -- before any deck
	// entry has touched the window at all.
	preEntry, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry before any entry: %v", err)
	}
	if preEntry.WindowSizeSet {
		t.Fatalf("test assumption violated: window-size already set window-locally before any entry: %+v", preEntry)
	}

	// Neither deck option exists yet either.
	if _, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("test assumption violated: %s already present before any entry", tmux.OwnershipOption)
	}
	if _, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("test assumption violated: %s already present before any entry", tmux.IsizeGeometryOption)
	}

	// Entry (`\u21b5`, force=false): the first claim, at one preview size.
	next1, _ := newTestModel(100, 30).enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry did not enter interactive mode: attachError=%q", got1.attachError)
	}
	if got1.interactiveGeometry != preEntry {
		t.Fatalf("first entry's own restore target = %+v, want the pre-entry capture %+v", got1.interactiveGeometry, preEntry)
	}

	// Steal (`F`, force=true) at a DIFFERENT preview size: the second
	// claim, taking over got1's live claim.
	next2, _ := newTestModel(120, 40).enterInteractiveBody(true)
	got2 := next2.(Model)
	if got2.attachError != "" {
		t.Fatalf("first steal refused: %q", got2.attachError)
	}
	if !got2.interactive {
		t.Fatalf("first steal did not enter interactive mode")
	}
	if got2.interactiveGeometry != preEntry {
		t.Fatalf("first steal's own restore target = %+v, want the adopted original %+v (not re-captured)", got2.interactiveGeometry, preEntry)
	}

	// Steal AGAIN (`F`, force=true) at yet another preview size: the
	// THIRD claim, taking over got2's live claim. This is the "steal →
	// steal" chain the task names.
	next3, _ := newTestModel(140, 45).enterInteractiveBody(true)
	got3 := next3.(Model)
	if got3.attachError != "" {
		t.Fatalf("second steal refused: %q", got3.attachError)
	}
	if !got3.interactive {
		t.Fatalf("second steal did not enter interactive mode")
	}
	if got3.interactiveGeometry != preEntry {
		t.Fatalf("second steal's own restore target = %+v, want the adopted original %+v (not re-captured)", got3.interactiveGeometry, preEntry)
	}

	// The LEGITIMATE exit: only the final, never-stolen-from holder ever
	// calls exitInteractive. got1 and got2 were both stolen from and, per
	// task 112's still-mine gate, are never trusted to tear anything down
	// here (exactly like teardown_still_mine_gate_test.go's own got1).
	got3.exitInteractive()

	postExit, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry after the legitimate exit: %v", err)
	}
	if postExit != preEntry {
		t.Fatalf("post-exit geometry = %+v, want the pre-first-entry capture %+v byte-exact", postExit, preEntry)
	}

	if _, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("%s still present after the legitimate exit, want unset", tmux.OwnershipOption)
	}
	if _, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("%s still present after the legitimate exit, want unset", tmux.IsizeGeometryOption)
	}
}
