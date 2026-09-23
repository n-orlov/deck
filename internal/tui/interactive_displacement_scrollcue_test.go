// interactive_displacement_scrollcue_test.go is B3's own regression test
// (PRD phase4b review finding B3): a fallback reseed's own displacement
// notice must never manufacture scrollback that the R133 scroll cue then
// mistakes for a real scrolled-back position.
//
// internal/interactive/grid.go's fallbackLoop reseeds the grid from a
// VISIBLE-SCREEN-ONLY CaptureSeed (real scrollback 0 right after the
// reseed -- CaptureSeed's own doc, TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty),
// then writes pipeDisplacedNotice into that same grid so the panel says
// it fell back. Writing "\r\n...\r\n" at the bottom of an already-full
// screen is ordinary terminal output as far as vt is concerned, and vt
// scrolls the screen exactly like any other overflowing write -- pushing
// rows of the just-captured content into scrollback that has nothing to
// do with the pane's real history. Before the cure that artificial
// scrollback survived the reseed, and the stored interactive scroll
// offset -- healed against the grid's real (but now inflated) scrollback
// length -- landed above 0, so the footer kept showing a "not live"
// scroll cue even once the operator's own pre-displacement scroll
// position had nothing real left underneath it.
//
// This test drives the whole path end to end against a real tmux server,
// the same discipline interactive_footer_scroll_cue_test.go and
// interactive_displacement_test.go both already use: a history-bearing
// pane, a real scroll back into that history, a real second
// `pipe-pane -IO` displacing deck's own pipe (exactly
// internal/interactive/displacement_test.go's own mechanism, proved at
// the Session level), a wait for the fallback to actually reseed at
// least once (proved the same non-vacuous way
// TestSessionFallsBackToPassiveCaptureWhenPipeIsDisplaced proves it:
// new pane output only shows up once a fresh capture tick has run), and
// then all five of B3's own assertions against the real Model.
package tui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func TestDisplacementFallbackReseedLeavesNoArtificialScrollbackOrCue(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	const (
		printed    = 400
		slug       = "dispb3"
		tmuxSess   = "deck_dispb3"
		historyLen = printed
	)
	socket := selectionTestSocket("dispb3")
	newShellPaneWithHistory(t, socket, tmuxSess, 80, 10, printed)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-dispb3-1", Name: slug, Slug: slug, Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil {
		t.Fatalf("enterInteractive did not enter interactive mode with a live grid")
	}
	defer got.exitInteractive()

	realScrollbackLen := got.interactiveGrid.Grid().ScrollbackLen()
	if realScrollbackLen <= 0 {
		t.Fatalf("test fixture assumption broken: the pane's grid holds %d scrollback lines, want > 0", realScrollbackLen)
	}

	// Scroll back into the pane's REAL history, exactly like an operator
	// pressing PgUp/wheel-up would, before anything ever displaces the
	// pipe.
	scrolled, _ := got.scrollInteractiveByLines(realScrollbackLen)
	got, ok = scrolled.(Model)
	if !ok {
		t.Fatalf("scrollInteractiveByLines returned a non-Model tea.Model")
	}
	if got.interactiveScrollOffset() <= 0 {
		t.Fatalf("test setup: scrolling back %d lines left the stored offset at %d, want > 0", realScrollbackLen, got.interactiveScrollOffset())
	}

	// Displace deck's own pipe with a second `pipe-pane -IO` on the SAME
	// target -- internal/interactive/displacement_test.go's own mechanism
	// for the same finding, one level down.
	if out, err := exec.Command("tmux", "-L", socket, "pipe-pane", "-IO", "-t", tmuxSess, "cat > /dev/null").CombinedOutput(); err != nil {
		t.Fatalf("arm the displacing pipe-pane: %v: %s", err, out)
	}

	if !waitForDisplacementTest(t, 2*time.Second, func() bool {
		return got.interactiveGrid.Status() == interactive.StatusDisplaced
	}) {
		t.Fatalf("grid never reported StatusDisplaced within 2s of the pipe being displaced")
	}

	// Non-vacuity: prove the fallback has actually RESEEDED at least once
	// (not merely written its one-shot notice into the pre-displacement
	// grid) by sending brand-new pane output and waiting for a passive
	// capture snapshot to pick it up -- nothing else in this Session can
	// make new pane output appear in the grid once the pipe is gone.
	const fallbackMarker = "B3-FALLBACK-RESEEDED"
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", tmuxSess, "-l", "--", "echo "+fallbackMarker).CombinedOutput(); err != nil {
		t.Fatalf("send-keys -l -- echo %s: %v: %s", fallbackMarker, err, out)
	}
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", tmuxSess, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("send-keys Enter: %v: %s", err, out)
	}

	_, height := m.previewContentSize()
	if !waitForDisplacementTest(t, 2*time.Second, func() bool {
		sbLen := got.interactiveGrid.Grid().ScrollbackLen()
		rows, _ := got.interactiveGrid.RenderRows(0, sbLen+height)
		return entryHistoryRowsContain(rows, fallbackMarker)
	}) {
		t.Fatalf("fallback passive-capture snapshots never picked up pane output written after displacement -- the reseed this test's other assertions depend on never happened")
	}

	// B3's own five assertions, against the fallback-reseeded Model.
	if got.interactiveGrid.Status() != interactive.StatusDisplaced {
		t.Fatalf("Status() = %v after the fallback reseed, want StatusDisplaced", got.interactiveGrid.Status())
	}
	if sbLen := got.interactiveGrid.Grid().ScrollbackLen(); sbLen != 0 {
		t.Fatalf("grid's scrollback length = %d after the fallback reseed, want 0: the displacement notice's own write must never leave artificial scrollback behind", sbLen)
	}

	// A plain render heals the stored offset against the grid's real
	// current scrollback length even with no Update in between
	// (interactive_scroll.go's own doc for healInteractiveScrollOffsetFromRender);
	// call it through the real product chain (View(), never the heal
	// helper directly) so this proves the same thing an operator's own
	// next frame would see.
	view := got.View()

	if got.interactiveScrollOffset() != 0 {
		t.Fatalf("stored interactive scroll offset = %d after the fallback reseed, want 0", got.interactiveScrollOffset())
	}

	footer := footerLineOf(view)
	if strings.Contains(strings.ToLower(footer), "not live") {
		t.Fatalf("footer = %q, carries a \"not live\" scroll cue after the fallback reseed even though the reseeded grid has no real scrollback left under the healed offset", footer)
	}

	sbLen := got.interactiveGrid.Grid().ScrollbackLen()
	rows, _ := got.interactiveGrid.RenderRows(0, sbLen+height)
	if !entryHistoryRowsContain(rows, "displaced") {
		t.Fatalf("the displacement notice is no longer visible in the panel after the fallback reseed")
	}
}
