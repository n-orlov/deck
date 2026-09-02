package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// readWindowIsizeGeometryOptionForTest reads tmux.IsizeGeometryOption's
// raw value off target on socket, the same "invalid option" == unset
// distinction internal/tmux/isize_geometry.go's own
// readWindowIsizeGeometry makes internally -- mirrored here because that
// helper is unexported and this package cannot reach it directly, exactly
// like force_indistinguishable_test.go's own
// readWindowOwnershipOptionForTest does for OwnershipOption.
func readWindowIsizeGeometryOptionForTest(t *testing.T, socket, target string) (string, bool) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-options", "-wv", "-t", target, tmux.IsizeGeometryOption).CombinedOutput()
	trimmed := strings.TrimRight(string(out), "\n")
	if err != nil {
		if strings.Contains(trimmed, "invalid option") {
			return "", false
		}
		t.Fatalf("show-options -wv -t %s %s: %v: %s", target, tmux.IsizeGeometryOption, err, trimmed)
	}
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// TestFirstEntryWritesIsizeGeometryThenStealAdoptsWithoutRewrite proves
// task 111's whole point end to end, for both entry paths sharing
// enterInteractiveBody: the FIRST entry onto a window (`\u21b5`, force=false)
// writes @deck_isize_geometry from its own CaptureWindowGeometry because
// the option is absent on this fresh window, and a SECOND entry (`F`,
// force=true) that steals the live claim reads -- never rewrites -- that
// already-recorded value: the option's raw wire string is byte-identical
// before and after the steal, and the stealing model's own restore target
// (interactiveGeometry) equals the parsed original, not a fresh capture
// of the first holder's now-fitted size.
func TestFirstEntryWritesIsizeGeometryThenStealAdoptsWithoutRewrite(t *testing.T) {
	socket := selectionTestSocket("isizefirstwrite")
	newQuietSelectionPane(t, socket, "deck_isizefirstwrite", 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("isizefirstwrite")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	newTestModel := func() Model {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = 100, 30
		if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
		}
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-isize-1", Name: "isizefirstwrite", Slug: "isizefirstwrite", Status: "waiting"}}
		m.selected = 0
		return m
	}

	// Before either entry, the option must not exist at all -- otherwise
	// this test would not be exercising the "absent" branch it claims to.
	if _, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget); ok {
		t.Fatalf("test assumption violated: %s already present before any entry", tmux.IsizeGeometryOption)
	}

	wantOriginal, err := client.CaptureWindowGeometry(context.Background(), windowTarget)
	if err != nil {
		t.Fatalf("CaptureWindowGeometry (test's own pre-entry snapshot): %v", err)
	}

	// First entry (`\u21b5`, force=false): the option is absent, so this
	// entry must write it from its own capture.
	next1, _ := newTestModel().enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry (force=false) did not enter interactive mode: attachError=%q", got1.attachError)
	}
	if got1.interactiveGeometry != wantOriginal {
		t.Fatalf("first entry's own restore target = %+v, want the pre-entry capture %+v", got1.interactiveGeometry, wantOriginal)
	}

	rawBefore, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s was not written by the first entry", tmux.IsizeGeometryOption)
	}
	parsedBefore, parsedOK := tmux.ParseIsizeGeometry(rawBefore)
	if !parsedOK {
		t.Fatalf("%s = %q did not parse", tmux.IsizeGeometryOption, rawBefore)
	}
	if parsedBefore != wantOriginal {
		t.Fatalf("%s parsed = %+v, want the pre-entry capture %+v", tmux.IsizeGeometryOption, parsedBefore, wantOriginal)
	}

	// Second entry (`F`, force=true): steals got1's live claim. The window
	// is now fitted to got1's own preview box, so a re-capture at this
	// point would see got1's fitted size, not the original -- the very
	// hazard R100 exists to avoid.
	next2, _ := newTestModel().enterInteractiveBody(true)
	got2 := next2.(Model)
	if got2.attachError != "" {
		t.Fatalf("second entry (force=true) refused despite a live holder to steal from: %q", got2.attachError)
	}
	if !got2.interactive {
		t.Fatalf("second entry (force=true) did not enter interactive mode")
	}

	rawAfter, ok := readWindowIsizeGeometryOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s vanished after the steal", tmux.IsizeGeometryOption)
	}
	if rawAfter != rawBefore {
		t.Fatalf("%s changed across the steal: before=%q after=%q, want byte-identical (read-never-rewrite)", tmux.IsizeGeometryOption, rawBefore, rawAfter)
	}
	if got2.interactiveGeometry != parsedBefore {
		t.Fatalf("stealer's own restore target = %+v, want the parsed original %+v (adopted, not re-captured)", got2.interactiveGeometry, parsedBefore)
	}

	// Clean up through the surviving claim only: got1 was stolen from, so
	// its own exitInteractive (task 112's job to gate, not this task's)
	// must not be trusted to tear anything down correctly here.
	got2.exitInteractive()
}
