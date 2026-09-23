package tui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// entryHistoryMarker prefixes every line the fixture pane prints BEFORE
// interactive mode is ever entered, so a row found in the grid can be
// attributed to the pane's pre-entry output and to nothing else.
const entryHistoryMarker = "entry-hist"

// entryHistoryLine formats line n the way the fixture's own shell loop
// prints it. The ZERO-PADDED "%04d" is the load-bearing part, and it is
// the same discipline internal/interactive/seedhistory_test.go's
// seedHistoryLine and internal/tmux/paneseed_history_test.go follow, for
// two reasons that both bite here:
//
//  1. every needle is searched for as a SUBSTRING of a rendered row, and
//     the unpadded "entry-hist-1" is a substring of "entry-hist-10"
//     through "entry-hist-19" and of "entry-hist-100" through
//     "entry-hist-199" -- all of which this 400-line fixture really
//     prints. So "the grid contains entry-hist-1" proved only that the
//     grid held SOME line in the first two hundred, which the visible
//     screen alone can easily satisfy; "entry-hist-0001" can only be the
//     one line the fixture printed first;
//  2. the shell ECHOES the command that produces these lines, so the
//     pane's visible screen always contains the literal format string.
//     "entry-hist-%04d" cannot be mistaken for a formatted instance,
//     whereas a bare "%d" form leaves the two closer together than is
//     comfortable.
func entryHistoryLine(n int) string {
	return fmt.Sprintf("%s-%04d", entryHistoryMarker, n)
}

// newShellPaneWithHistory starts a real tmux session on a private socket
// whose pane has REAL tmux-side scrollback: a shell prints `lines`
// numbered lines into a pane far too short to hold them, so all but the
// last screenful is history and nothing else, and then parks in
// `cat > /dev/null` so the pane is quiescent by the time the test enters
// interactive mode (a chatty pane would make the entry seed's own
// before/after probes disagree and cost retries for no benefit here).
//
// This is deliberately NOT newQuietSelectionPane (which runs `sleep 600`,
// a pane that can never print anything and therefore can never have
// history) -- issue #29 is a claim about output produced before the
// preview was ever opened, which needs a pane that really produced some.
// The loop is a POSIX `while` rather than `seq`, so it needs nothing
// beyond the /bin/sh tmux falls back to inside ci/run.sh's container, the
// same reasoning internal/interactive's own fillPaneHistory records.
func newShellPaneWithHistory(t *testing.T, socket, session string, width, height, lines int) {
	t.Helper()
	create := []string{
		"-L", socket, "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height),
	}
	if out, err := exec.Command("tmux", create...).CombinedOutput(); err != nil {
		t.Fatalf("start shell tmux session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})

	// "%04d" here must stay in lockstep with entryHistoryLine's own
	// formatting -- see its doc for why the padding is what makes a needle
	// like "entry-hist-0001" mean one specific line rather than any of two
	// hundred.
	cmd := "i=0; while [ $i -lt " + strconv.Itoa(lines) + " ]; do i=$((i+1)); printf '" + entryHistoryMarker + "-%04d\\n' $i; done; cat > /dev/null"
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", session, "-l", "--", cmd).CombinedOutput(); err != nil {
		t.Fatalf("send-keys -l -- %q: %v: %s", cmd, err, out)
	}
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", session, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("send-keys Enter: %v: %s", err, out)
	}

	// Wait for the history to actually exist rather than sleeping a fixed
	// span and hoping: #{history_size} is the very quantity the entry seed
	// is about, so it is also the honest readiness signal. `lines - height`
	// is the floor the loop must reach once a screenful is still on screen.
	deadline := time.Now().Add(15 * time.Second)
	want := lines - height
	for {
		out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", session, "#{history_size}").Output()
		if err != nil {
			t.Fatalf("read #{history_size}: %v", err)
		}
		size, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
		if convErr != nil {
			t.Fatalf("parse #{history_size} %q: %v", strings.TrimSpace(string(out)), convErr)
		}
		if size >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane history reached only %d lines within the deadline, want at least %d -- the fixture never produced the scrollback this test is about", size, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// entryHistoryRowsContain reports whether needle appears on any of rows,
// once each row's own styling escapes are stripped. Rows are searched
// individually and never concatenated, so a needle can only match text
// that really is on one row -- the same discipline
// internal/interactive's gridContains follows for the same reason.
func entryHistoryRowsContain(rows []string, needle string) bool {
	for _, row := range rows {
		if strings.Contains(interactiveRepaintAnsiEscapeRe.ReplaceAllString(row, ""), needle) {
			return true
		}
	}
	return false
}

// TestEnterInteractiveSeedsTheGridWithThePanesOwnTmuxHistory is issue
// #29's regression test at the place the issue is actually reported from:
// the ENTRY path, driven through the real Model.enterInteractive against a
// real tmux server, not through internal/interactive's seed helpers
// directly.
//
// The symptom it pins: a session runs for a while with no preview open,
// the operator selects its row and presses Enter, and Shift+PgUp/wheel-up
// reaches NOTHING -- because everything the pane printed before entry was
// never written through deck's grid, so the grid's 2000-line scrollback
// (interactive.ScrollbackMaxLines) starts empty even though tmux itself
// still holds every one of those rows. The fix is the entry seed's range:
// interactive.CaptureSeedWithHistory over
// tmux.SeedCaptureOptionsWithHistory, in place of the visible-screen-only
// interactive.CaptureSeed the closure used to call.
//
// What makes this test load-bearing rather than decorative is the
// NON-VACUITY control below: it first proves the marker row it looks for
// is NOT on the live screen at all. Without that, "the grid contains
// entry-hist-1" would pass just as happily against a visible-only seed
// that happened to have the line on screen, and the guard would be
// worthless. Both halves come off the SAME rendered grid, through the
// same RenderRows the preview panel itself paints with
// (interactiveBodyLines), so what is asserted is what a user scrolling
// back would really see.
func TestEnterInteractiveSeedsTheGridWithThePanesOwnTmuxHistory(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	// Enough lines to be unmistakably scrollback, and to stay so after the
	// entry's own FitWindowToPane grows the pane from 10 rows to the
	// preview box's height (a taller pane pulls rows back OUT of tmux's
	// history, which is exactly why the fixture prints far more than one
	// screenful rather than just a few rows over).
	const printed = 400
	socket := selectionTestSocket("entryhist")
	newShellPaneWithHistory(t, socket, "deck_entryhist", 80, 10, printed)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-entryhist-1", Name: "entryhist", Slug: "entryhist", Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil {
		t.Fatalf("enterInteractive did not enter interactive mode (interactive=%v, grid=%v)", got.interactive, got.interactiveGrid == nil)
	}
	defer got.exitInteractive()

	grid := got.interactiveGrid.Grid()
	screenHeight := grid.Height()
	sbLen := grid.ScrollbackLen()
	if sbLen <= 0 {
		t.Fatalf("the grid's scrollback holds %d lines immediately after entry, want > 0: this IS issue #29's symptom -- the entry seed pulled the visible screen only, so Shift+PgUp/wheel-up has nothing to reach", sbLen)
	}
	if sbLen > interactive.ScrollbackMaxLines {
		t.Errorf("the grid's scrollback holds %d lines, more than its own bound %d (PRD II-51): the entry seed must not be able to grow it past the bound", sbLen, interactive.ScrollbackMaxLines)
	}

	// The live view -- exactly what interactiveBodyLines paints at scroll
	// offset 0 -- and then the whole grid, scrollback included, which is
	// what Shift+PgUp/wheel-up walks through.
	live, _ := got.interactiveGrid.RenderRows(0, screenHeight)
	everything, _ := got.interactiveGrid.RenderRows(0, sbLen+screenHeight)

	// The FIRST line the pane ever printed: hundreds of rows above the
	// live screen, so it can only be present if the seed read tmux's own
	// history.
	oldest := entryHistoryLine(1)
	if entryHistoryRowsContain(live, oldest) {
		t.Fatalf("test assumption violated: %q is on the LIVE screen (pane height %d, %d lines printed), so finding it in the grid would prove nothing about history at all", oldest, screenHeight, printed)
	}
	if !entryHistoryRowsContain(everything, oldest) {
		t.Errorf("%q is absent from the grid's %d scrollback lines + %d screen rows, want present: the entry seed must include the pane's tmux history (interactive.CaptureSeedWithHistory), not just its visible screen", oldest, sbLen, screenHeight)
	}
}

// TestInteractiveEntrySeedAsksForTheTransportsOwnHistoryBound is the
// mechanical half of the same guard, and covers the one thing the behavioural
// test above structurally cannot see: HOW MUCH history the entry seed asks
// tmux for. A closure that asked for, say, 50 lines would still make
// TestEnterInteractiveSeedsTheGridWithThePanesOwnTmuxHistory pass (50 is more
// than nothing), while quietly leaving 1950 of the grid's own 2000 scrollable
// lines empty -- a partial, silent revert of issue #29 that no observation of
// a single pane's grid can distinguish from a pane that simply had less
// history than that.
//
// The right amount is now TRANSPORT-DEPENDENT, which is why this guard no
// longer looks for interactive.ScrollbackMaxLines at the call site: under
// TransportPipe the entry seed is the only thing that ever writes the pane's
// pre-entry output into the grid, so it must ask for everything the grid can
// hold; under TransportCapture captureLoop replaces the whole grid from a
// visible-only capture within one poll interval, so history pulled at entry
// is thrown away 200ms later and asking for it only buys a hitch on
// bubbletea's Update goroutine. internal/interactive.EntrySeedHistoryLines is
// where that decision lives and where both measurements are recorded.
//
// So the guard holds three facts, the first two behavioural and the third
// structural:
//
//  1. the amount the pipe path resolves to IS the grid's whole bound, and the
//     amount the capture path resolves to is NONE -- asserted by calling the
//     same expression the call site passes, so a change to either answer
//     fails here as well as in internal/interactive's own test;
//  2. the two config strings internal/tui itself maps to those transports
//     ("" / "pipe" and "capture", enterInteractiveBody's own switch) really
//     land on the two different answers, so a call site that ignored the
//     transport could not pass;
//  3. this package's non-test source has exactly ONE entry-seed call site and
//     it passes interactive.EntrySeedHistoryLines(transport) -- not a literal,
//     not interactive.ScrollbackMaxLines unconditionally (issue #29's first
//     shape, which made the capture transport pay for history it discards),
//     and not the visible-screen-only interactive.CaptureSeed. The scan is
//     the same technique internal/interactive's own
//     TestExactlyOneGridConstructorCallSite and TestNoCodePathCallsGridResize-
//     Alone use, and it is what makes fact 1 mean something about the product
//     rather than about the helper in isolation.
//
// Demonstrate-then-revert: change the call in interactive.go back to
// interactive.CaptureSeed(ctx, client, pane.ID), or to
// interactive.CaptureSeedWithHistory(..., interactive.ScrollbackMaxLines),
// and rerun -- fact 3 goes red naming the file -- then restore it.
func TestInteractiveEntrySeedAsksForTheTransportsOwnHistoryBound(t *testing.T) {
	// Fact 1: the two answers themselves, read through the same call the
	// entry closure makes.
	if got := interactive.EntrySeedHistoryLines(interactive.TransportPipe); got != interactive.ScrollbackMaxLines {
		t.Errorf("the entry seed asks for %d lines of history under the pipe transport, want interactive.ScrollbackMaxLines (%d): under pipe the entry seed is the ONLY thing that ever writes the pane's pre-entry output into the grid, so anything less silently leaves part of the grid's scrollable range empty (issue #29)", got, interactive.ScrollbackMaxLines)
	}
	if got := interactive.EntrySeedHistoryLines(interactive.TransportCapture); got != 0 {
		t.Errorf("the entry seed asks for %d lines of history under the capture transport, want 0: captureLoop replaces the whole grid from a visible-only capture within one poll interval, so every history row pulled here is discarded ~200ms later -- paid for with a hitch on bubbletea's blocking Update goroutine at the exact moment the operator pressed Enter", got)
	}

	// Fact 2: enterInteractiveBody's OWN config-string-to-transport
	// mapping, which is what decides which of Fact 1's two answers a real
	// entry gets. This is asserted against the source rather than by
	// re-deriving the mapping in the test: a table here that wrote
	// `if setting == "capture" { transport = TransportCapture }` for
	// itself would pass just as happily with that branch inverted or
	// deleted in interactive.go, which is precisely the regression worth
	// catching -- an inverted mapping silently swaps the two costs, giving
	// the pipe transport an empty scrollback (issue #29 reverted in
	// practice) and the capture transport the ~100ms hitch for history it
	// discards 200ms later.
	transportMappingRe := regexp.MustCompile(`transport := interactive\.TransportPipe\s*\n\s*if m\.settings\.InteractiveTransport == "capture" \{\s*\n\s*transport = interactive\.TransportCapture\s*\n\s*\}`)
	entrySource, err := os.ReadFile("interactive.go")
	if err != nil {
		t.Fatalf("ReadFile(internal/tui/interactive.go): %v", err)
	}
	if !transportMappingRe.Match(entrySource) {
		t.Errorf("interactive.go no longer maps interactive_transport to a transport in the shape this test recognises (default TransportPipe, and TransportCapture only for the literal \"capture\"). If the mapping was deliberately restructured, re-aim this assertion at the new shape rather than deleting it: it is the only thing pinning WHICH of EntrySeedHistoryLines' two answers a real entry receives, and inverting it reverts issue #29 for the default transport while making the other one pay for history it throws away")
	}

	// Fact 3: the one call site, and what it passes.
	entrySeedRe := regexp.MustCompile(`interactive\.CaptureSeedWithHistory\(ctx, client, pane\.ID, interactive\.EntrySeedHistoryLines\(transport\)\)`)
	visibleOnlyRe := regexp.MustCompile(`interactive\.CaptureSeed\(`)
	unconditionalBoundRe := regexp.MustCompile(`interactive\.CaptureSeedWithHistory\([^)]*interactive\.ScrollbackMaxLines\)`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/tui): %v", err)
	}

	entrySeedSites := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		entrySeedSites += len(entrySeedRe.FindAllIndex(data, -1))
		if visibleOnlyRe.Match(data) {
			t.Errorf("%s calls interactive.CaptureSeed, the VISIBLE-SCREEN-ONLY seed: internal/tui owns only the one-off ENTRY seed, and issue #29 requires that one to be history-inclusive (interactive.CaptureSeedWithHistory). CaptureSeed belongs to internal/interactive's own 200ms reseed loops, where the history-inclusive range was measured too expensive to run per tick", name)
		}
		if unconditionalBoundRe.Match(data) {
			t.Errorf("%s asks the entry seed for interactive.ScrollbackMaxLines UNCONDITIONALLY: that is issue #29's first shape, and it makes the capture transport pay a one-off ~110ms Update-goroutine hitch for history captureLoop throws away at its first poll tick. Pass interactive.EntrySeedHistoryLines(transport) and let internal/interactive own the answer", name)
		}
	}
	if entrySeedSites != 1 {
		t.Fatalf("internal/tui has %d interactive.CaptureSeedWithHistory(ctx, client, pane.ID, interactive.EntrySeedHistoryLines(transport)) call sites in its non-test source, want exactly 1 (the entry seed closure in interactive.go's enterInteractiveBody, issue #29)", entrySeedSites)
	}
}
