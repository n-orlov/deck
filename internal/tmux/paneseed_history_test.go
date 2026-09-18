// paneseed_history_test.go pins the tmux behaviours issue #29's
// history-inclusive entry seed rests on, every one of them against a real
// tmux server rather than an assumption: `capture-pane -S -<N>` clamps a
// range longer than the pane's actual history instead of erroring, an
// alternate-screen pane's stale primary-screen history is suppressed
// before it can reach a seed, and tmux reflows history on a narrowing
// resize so that a captured row never exceeds the pane's CURRENT width.
package tmux

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

// historyMarker is the text every history-producing fixture below prints,
// one numbered line at a time. The "%04d" formatting matters: the shell
// ECHOES the command that produces these lines, so the pane's own visible
// screen always contains the literal string "historyMarker-%04d" from that
// echo. Searching for a FORMATTED instance ("hist-mark-0001") therefore
// only ever matches real produced output, never the command echo -- which
// is exactly the distinction the alt-screen assertions below depend on.
const historyMarker = "hist-mark"

// fillPaneHistory makes target produce `lines` numbered lines of output,
// enough of which scroll off the top to give the pane real tmux-side
// scrollback, and waits for tmux to have processed all of it. The command
// is a POSIX `while` loop rather than `seq`, so it needs nothing beyond
// the /bin/sh tmux falls back to inside ci/run.sh's container.
//
// Unlike paneseed_test.go's runInPaneBlocking this deliberately does NOT
// leave a blocking `cat` running: several tests below need to send a
// SECOND command to the same pane afterwards (a `clear`, an alt-screen
// switch), and a pane parked in `cat` would swallow it as stdin. The pane
// is quiescent all the same once the loop has finished -- the shell is
// sitting at a prompt printed on the pane's last row, which is not
// itself a scroll -- so the before/after discriminator probes of an
// atomic capture agree on the first attempt.
func fillPaneHistory(t *testing.T, socket, target string, lines int) {
	t.Helper()
	cmd := "i=0; while [ $i -lt " + strconv.Itoa(lines) + " ]; do i=$((i+1)); printf '" + historyMarker + "-%04d\\n' $i; done"
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", cmd)
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
	time.Sleep(500 * time.Millisecond)
}

// paneHistoryAndHeight reads #{history_size} and #{pane_height} in ONE
// display-message, so the two can never describe different moments of a
// pane's life.
func paneHistoryAndHeight(t *testing.T, socket, target string) (history, height int) {
	t.Helper()
	out := runTmux(t, socket, "display-message", "-p", "-t", target, "#{history_size}|#{pane_height}")
	fields := strings.Split(strings.TrimSpace(out), "|")
	if len(fields) != 2 {
		t.Fatalf("display-message history/height returned %q, want two |-joined fields", out)
	}
	var err error
	if history, err = strconv.Atoi(fields[0]); err != nil {
		t.Fatalf("parse history_size %q: %v", fields[0], err)
	}
	if height, err = strconv.Atoi(fields[1]); err != nil {
		t.Fatalf("parse pane_height %q: %v", fields[1], err)
	}
	return history, height
}

// captureRowCount counts the ROWS in a capture-pane body. Row count is
// newline count, not newline count plus one: `capture-pane -N` terminates
// every row it prints, its last one included (the same property
// internal/interactive/seed.go's step-3 comment turns on), so a body of R
// rows carries exactly R newlines -- the R-1 separators plus the
// terminator.
func captureRowCount(body []byte) int {
	return bytes.Count(body, []byte("\n"))
}

// TestSeedCaptureOptionsWithHistoryClampsToAvailableHistory is what
// protects the entire "-S -2000 just works" assumption
// SeedCaptureOptionsWithHistory is built on, for whatever tmux the suite
// happens to run against (>= 3.2 by MinimumMajor/MinimumMinor; 3.5a in
// ci/run.sh's container, newer on a developer host).
//
// The contract, measured: `capture-pane -N -S -<N> -E -` returns exactly
// min(history_size, N) + pane_height rows, and CLAMPS rather than
// erroring when the pane's history is shorter than N. Nothing in deck
// pre-reads #{history_size} to size the range -- that read would race the
// capture anyway -- so if a future tmux stopped clamping (an error, a
// short read, or padding the missing rows out to N) the seed would
// silently misalign: too few rows and the live screen would sit too high
// in the grid, too many and it would be pushed into scrollback. This test
// makes that a loud failure here instead.
func TestSeedCaptureOptionsWithHistoryClampsToAvailableHistory(t *testing.T) {
	socket := geometrySocket("seed-history-clamp")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	fillPaneHistory(t, socket, "s0", 50)
	paneID := resolveSolePaneID(t, socket, "s0")
	history, height := paneHistoryAndHeight(t, socket, "s0")
	if history <= 0 {
		t.Fatalf("fixture produced history_size %d; the rest of this test is vacuous without real scrollback", history)
	}

	// Case 1: a range far LONGER than the pane's history -- the
	// production case, since the entry seed asks for ScrollbackMaxLines
	// (2000) whenever it asks for history at all
	// (interactive.EntrySeedHistoryLines, which answers 2000 under the
	// pipe transport and 0 under capture). tmux must clamp to what exists.
	long, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane with a 2000-line history range: %v (want a graceful clamp, not an error)", err)
	}
	if got, want := captureRowCount(long), history+height; got != want {
		t.Errorf("history-inclusive capture over a 2000-line range returned %d rows, want %d (history_size %d + pane_height %d): tmux no longer clamps an over-long -S to the available history", got, want, history, height)
	}

	// Case 2: a range SHORTER than the pane's history, which proves the
	// arithmetic above is min(history, N) rather than "always everything".
	const shortRange = 5
	if shortRange >= history {
		t.Fatalf("fixture history_size %d is not greater than the short range %d; case 2 would not distinguish the two", history, shortRange)
	}
	short, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(shortRange))
	if err != nil {
		t.Fatalf("CapturePane with a %d-line history range: %v", shortRange, err)
	}
	if got, want := captureRowCount(short), shortRange+height; got != want {
		t.Errorf("history-inclusive capture over a %d-line range returned %d rows, want %d (%d + pane_height %d)", shortRange, got, want, shortRange, height)
	}

	// Case 3: NO history at all. `clear` emits ESC[3J, on which tmux
	// zeroes the pane's history outright (which is also why
	// #{history_size} is not monotonic and why the atomic capture's retry
	// can genuinely be exhausted -- see
	// interactive.CaptureSeedWithHistory's degradation path). The pane is
	// then in the state a freshly launched session is in, the single most
	// common target of an entry seed, and the capture must come back with
	// exactly the visible screen and no error.
	runTmux(t, socket, "send-keys", "-t", "s0", "-l", "--", "clear")
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	time.Sleep(500 * time.Millisecond)
	clearedHistory, clearedHeight := paneHistoryAndHeight(t, socket, "s0")
	if clearedHistory != 0 {
		t.Fatalf("after `clear` history_size = %d, want 0; this tmux does not zero history on ESC[3J, so case 3's premise does not hold", clearedHistory)
	}
	empty, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane with a 2000-line history range against an EMPTY history: %v (want a graceful clamp, not an error)", err)
	}
	if got := captureRowCount(empty); got != clearedHeight {
		t.Errorf("history-inclusive capture of an empty-history pane returned %d rows, want exactly pane_height (%d)", got, clearedHeight)
	}
}

// TestSeedCaptureOptionsWithHistoryStartLineAndDegenerateValues pins the
// options themselves, so the two ranges cannot silently converge: the
// periodic reseed loops MUST stay on the visible screen (see
// SeedCaptureOptions' own doc for the measured per-tick cost that decides
// it), and a non-positive historyLines must mean exactly that same
// visible-only range rather than a nonsensical "-0"/"0" start line.
func TestSeedCaptureOptionsWithHistoryStartLineAndDegenerateValues(t *testing.T) {
	if got := SeedCaptureOptions().StartLine; got != "0" {
		t.Errorf("SeedCaptureOptions().StartLine = %q, want %q (the visible screen only -- the periodic reseed loops depend on this range staying cheap)", got, "0")
	}
	if got := SeedCaptureOptionsWithHistory(2000).StartLine; got != "-2000" {
		t.Errorf("SeedCaptureOptionsWithHistory(2000).StartLine = %q, want %q", got, "-2000")
	}
	// Everything except the start line must be identical, or the two
	// bodies would not be comparable at all (-e and -N are what make a
	// body a seed rather than a plain text dump).
	visible, history := SeedCaptureOptions(), SeedCaptureOptionsWithHistory(2000)
	visible.StartLine, history.StartLine = "", ""
	if visible != history {
		t.Errorf("SeedCaptureOptionsWithHistory differs from SeedCaptureOptions in more than StartLine: %+v vs %+v", history, visible)
	}
	for _, n := range []int{0, -1, -2000} {
		if got := SeedCaptureOptionsWithHistory(n); got != SeedCaptureOptions() {
			t.Errorf("SeedCaptureOptionsWithHistory(%d) = %+v, want exactly SeedCaptureOptions() %+v", n, got, SeedCaptureOptions())
		}
	}
	// captureLinePattern already accepts the negative start line, so no
	// validation change was needed for issue #29 -- assert that rather
	// than leave it to be discovered by a CapturePane error.
	if !captureLinePattern.MatchString("-2000") {
		t.Errorf("captureLinePattern rejects %q, so every history-inclusive capture would fail validation", "-2000")
	}
}

// TestCapturePaneSeedAtomicSuppressesAlternateScreenHistory is the
// alt-screen exclusion issue #29 requires, measured end to end against a
// REAL alternate-screen pane.
//
// The hazard is tmux's own (verified below, not assumed): while a pane
// shows its alternate screen, tmux keeps the PRIMARY screen's history
// untouched and `capture-pane -S -<N>` happily prepends it -- so a
// full-screen program's seed would carry rows of stale shell output that
// nothing, not even a real `tmux attach`, can scroll back to, because an
// alternate screen has no scrollback. (The second, independent reason for
// the exclusion lives on the other side of the seam: a grid switched to
// its alternate buffer bounds and renders only the PRIMARY screen's
// scrollback, so those rows would be both invisible and unbounded.)
//
// The same pane is asserted BEFORE the alt-screen switch to prove the
// suppression is not simply "the capture never had history": before the
// switch the identical call returns history + pane_height rows and does
// contain the pre-launch text; after it, exactly pane_height rows and
// none of it.
func TestCapturePaneSeedAtomicSuppressesAlternateScreenHistory(t *testing.T) {
	socket := geometrySocket("seed-history-altscreen")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	fillPaneHistory(t, socket, "s0", 50)
	paneID := resolveSolePaneID(t, socket, "s0")
	history, height := paneHistoryAndHeight(t, socket, "s0")
	if history <= 0 {
		t.Fatalf("fixture produced history_size %d; this test cannot distinguish suppression from an absent history", history)
	}
	needle := historyMarker + "-0001"

	// Before the switch: the ordinary primary-screen case, which MUST
	// keep its history (that is the whole feature).
	primaryState, primaryBody, err := client.CapturePaneSeedAtomic(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic (primary screen, history-inclusive): %v", err)
	}
	if primaryState.AlternateOn {
		t.Fatalf("AlternateOn = true before any alternate-screen switch; fixture assumption violated")
	}
	if got, want := captureRowCount(primaryBody), history+height; got != want {
		t.Errorf("primary-screen history-inclusive body = %d rows, want %d (history_size %d + pane_height %d)", got, want, history, height)
	}
	if !bytes.Contains(primaryBody, []byte(needle)) {
		t.Fatalf("primary-screen history-inclusive body does not contain %q; the alt-screen assertion below would prove nothing", needle)
	}

	// Switch to the alternate screen and park there, exactly the shape a
	// full-screen program (an editor, a pager, a TUI agent) leaves a pane
	// in. The blocking `cat` keeps the shell from redrawing a prompt and
	// switching straight back.
	runInPaneBlocking(t, socket, "s0", `\033[?1049h`)

	altState, altBody, err := client.CapturePaneSeedAtomic(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic (alternate screen, history-inclusive): %v", err)
	}
	if !altState.AlternateOn {
		t.Fatalf("AlternateOn = false after ESC[?1049h; fixture assumption violated (state %+v)", altState)
	}
	_, altHeight := paneHistoryAndHeight(t, socket, "s0")
	if got := captureRowCount(altBody); got != altHeight {
		t.Errorf("alternate-screen history-inclusive body = %d rows, want exactly pane_height (%d): the pane's stale primary-screen history was not suppressed", got, altHeight)
	}
	if bytes.Contains(altBody, []byte(needle)) {
		t.Errorf("alternate-screen history-inclusive body contains the pre-launch text %q, which the alternate screen never showed:\n%q", needle, altBody)
	}

	// Non-vacuity for the suppression itself: the raw capture-pane call,
	// with no suppression layer over it, DOES return that stale history
	// for the very same pane and options. Without this the test would
	// pass just as happily against a tmux that had already excluded it.
	raw, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane (alternate screen, history-inclusive): %v", err)
	}
	if !bytes.Contains(raw, []byte(needle)) {
		t.Fatalf("raw capture-pane of the alternate-screen pane does NOT contain %q, so tmux itself already excludes primary-screen history here and CapturePaneSeedAtomic's visible-only re-capture is untested by this fixture (tmux behaviour has changed; re-derive the exclusion before trusting it)", needle)
	}
	if got := captureRowCount(raw); got <= altHeight {
		t.Fatalf("raw capture-pane of the alternate-screen pane returned %d rows (<= pane_height %d), so there was nothing to suppress", got, altHeight)
	}
}

// countingTmuxShim writes an executable that appends one line per
// invocation to a log and then execs the REAL tmux with the same
// arguments, so a test can both drive a real tmux server and count how
// many separate tmux invocations a single Client call made. Client.Binary
// is the seam; every Client method routes through it (c.binary()), and
// `exec` rather than a subprocess keeps stdout/stderr and the exit status
// byte-for-byte what the caller would have seen without the shim.
//
// Counting invocations is the only way to observe the alt-screen
// RE-CAPTURE (issue #29) at all: it is a second `capture-pane` whose
// result, on a quiescent pane, is byte-identical to the first attempt's
// visible rows -- so no assertion on the returned body can distinguish
// "re-captured" from "captured once".
func countingTmuxShim(t *testing.T) (binary, logPath string) {
	t.Helper()
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("look up the real tmux binary: %v", err)
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "invocations")
	binary = filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nexec " + realTmux + " \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, logPath
}

// shimInvocations returns the argv lines countingTmuxShim has logged so
// far, oldest first. A missing log means zero invocations, not a failure:
// the shim only creates the file when it first runs.
func shimInvocations(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read tmux invocation log: %v", err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// TestCapturePaneSeedAtomicVisibleOnlyRangeNeedsNoAlternateScreenRecapture
// pins the OTHER half of the alt-screen suppression (issue #29): the
// re-capture is conditional on the requested range having been wider than
// the visible screen, so an alternate-screen pane captured on the
// visible-only range -- the range the two periodic reseed loops use, at
// 200ms -- must cost exactly ONE tmux invocation and come back untouched.
//
// The invocation count is what makes this test load-bearing rather than a
// restatement of the byte-identity test below. Its predecessor asserted
// only that the returned body equalled capture-pane's own output, which
// was true no matter what -- the Go-side truncation it was written to
// exercise never fired on this range either, so deleting that truncation
// outright left it passing. A count of 2 here would mean every reseed
// tick against a full-screen program (an editor, a pager, an agent TUI)
// silently doubled its tmux traffic; a count of 1 is the claim.
func TestCapturePaneSeedAtomicVisibleOnlyRangeNeedsNoAlternateScreenRecapture(t *testing.T) {
	socket := geometrySocket("seed-history-altscreen-noop")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	ctx := context.Background()

	fillPaneHistory(t, socket, "s0", 50)
	paneID := resolveSolePaneID(t, socket, "s0")
	runInPaneBlocking(t, socket, "s0", `\033[?1049h\033[HALT-SCREEN-CONTENT`)

	// The shim is installed only now, so the fixture's own tmux calls
	// (which go through runTmux/runInPaneBlocking, not through Client) are
	// not in the log and the count below is this one Client call's alone.
	shim, invocationLog := countingTmuxShim(t)
	client := Client{Binary: shim, Socket: socket, Timeout: 5 * time.Second}

	state, body, err := client.CapturePaneSeedAtomic(ctx, paneID, SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic (alternate screen, visible only): %v", err)
	}
	if invocations := shimInvocations(t, invocationLog); len(invocations) != 1 {
		t.Errorf("CapturePaneSeedAtomic on the VISIBLE-ONLY range made %d tmux invocations, want exactly 1: an alternate-screen pane already captured at the visible-only range has nothing to re-capture, and the two 200ms reseed loops run this path forever:\n%s", len(invocations), strings.Join(invocations, "\n"))
	}
	if !state.AlternateOn {
		t.Fatalf("AlternateOn = false after ESC[?1049h; fixture assumption violated")
	}
	want, err := client.CapturePane(ctx, paneID, SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePane (alternate screen, visible only): %v", err)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("alternate-screen visible-only body differs from capture-pane's own output:\n got %q\nwant %q", body, want)
	}
	if !bytes.Contains(body, []byte("ALT-SCREEN-CONTENT")) {
		t.Errorf("alternate-screen visible-only body lost the alternate screen's OWN content: %q", body)
	}
}

// paneWithColouredHistoryAboveAlternateScreen builds the ONE fixture
// shape that can catch the colour corruption a Go-side truncation causes,
// and it is deliberately narrow: every row of the pane's tmux-side
// history is painted with a NON-DEFAULT background (ESC[41m plus ESC[K, so
// the fill reaches the row's last cell), the pane then switches to its
// alternate screen with that same pen still active, and paints altText as
// a full-width bar on row 0.
//
// That is what makes the boundary interesting. `capture-pane -e` emits an
// SGR sequence only where the pen CHANGES from the previous cell, so with
// red running continuously from the last history row into the alt
// screen's first cell, tmux emits NO introducer at the history -> alt
// boundary: the alt row arrives as a bare glyph run that inherits its red
// background from the history row above. Cutting the history rows off in
// Go therefore produces a body whose first row has no background SGR at
// all and paints DEFAULT, which is precisely the five-byte,
// row-count-invisible corruption issue #29's first implementation
// shipped. Measured on tmux 3.6b at 30x8: the sliced body's row 0 was
// "ALT-SCREEN-ONLY-CONTENT..." where the visible-only capture's was
// "\x1b[41mALT-SCREEN-ONLY-CONTENT...".
//
// The trailing `cat > /dev/null` parks the pane so the shell never
// redraws a prompt and switches straight back off the alternate screen,
// and so the before/after discriminator probes agree on the first
// attempt.
func paneWithColouredHistoryAboveAlternateScreen(t *testing.T, socket, target string, historyRows int, altText string) {
	t.Helper()
	cmd := `printf "\033[41m"; ` +
		`i=0; while [ $i -lt ` + strconv.Itoa(historyRows) + ` ]; do i=$((i+1)); printf "` + historyMarker + `-%04d\033[K\n" $i; done; ` +
		`printf "\033[?1049h\033[H` + altText + `\033[K"; cat > /dev/null`
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", cmd)
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
	time.Sleep(800 * time.Millisecond)
}

// naiveLastRowsSlice is the WRONG implementation, kept here as the
// non-vacuity control: body cut in Go to its last n rows, exactly as
// capturePaneSeedAtomicOnce used to do before this test existed. It is
// never used in production code -- it exists so the test can show that
// the naive answer really does differ from the correct one for this
// fixture, which is the only way to know the fixture would have caught
// the bug.
func naiveLastRowsSlice(body []byte, n int) []byte {
	rows := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	if len(rows) > n {
		rows = rows[len(rows)-n:]
	}
	return []byte(strings.Join(rows, "\n") + "\n")
}

// TestCapturePaneSeedAtomicAlternateScreenHistoryRangeIsByteIdenticalToVisibleOnly
// is the test whose absence let issue #29's alt-screen suppression ship
// broken. The suppression's real contract is not "the body has
// pane_height rows" and not "the body no longer contains the stale text"
// -- both of which a Go-side slice satisfies perfectly -- but that an
// alternate-screen pane's seed body is EXACTLY the body tmux produces for
// the visible-only range, byte for byte. Only tmux can produce that,
// because only tmux re-derives the `-e` SGR stream from cell attributes
// and emits the introducers the new first row needs; a slice of an
// already-produced capture inherits nothing and loses them.
//
// Two controls keep it honest:
//
//	(1) the raw history-inclusive capture really is taller than the pane,
//	    so there was something to suppress at all;
//	(2) the naive Go slice of that raw capture -- the exact code this test
//	    retires -- is NOT byte-equal to the visible-only capture, and the
//	    difference is specifically an SGR introducer missing from row 0.
//	    Without (2) a fixture whose history happened to end on a default
//	    pen would pass against both implementations and prove nothing.
func TestCapturePaneSeedAtomicAlternateScreenHistoryRangeIsByteIdenticalToVisibleOnly(t *testing.T) {
	const width, height, historyRows = 30, 8, 24
	const altText = "ALT-SCREEN-ONLY-CONTENT"
	socket := geometrySocket("seed-history-altscreen-bytes")
	cleanup := newBareGeometrySession(t, socket, "s0", width, height)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	paneWithColouredHistoryAboveAlternateScreen(t, socket, "s0", historyRows, altText)
	paneID := resolveSolePaneID(t, socket, "s0")
	history, paneHeight := paneHistoryAndHeight(t, socket, "s0")
	if history <= 0 {
		t.Fatalf("fixture produced history_size %d; with no history above the alternate screen there is nothing for the suppression to do", history)
	}

	historyState, historyBody, err := client.CapturePaneSeedAtomic(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic (alternate screen, history-inclusive): %v", err)
	}
	if !historyState.AlternateOn {
		t.Fatalf("AlternateOn = false after ESC[?1049h; fixture assumption violated (state %+v)", historyState)
	}
	visibleState, visibleBody, err := client.CapturePaneSeedAtomic(ctx, paneID, SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic (alternate screen, visible-only): %v", err)
	}
	if !visibleState.AlternateOn {
		t.Fatalf("AlternateOn = false on the visible-only capture; the pane left its alternate screen mid-test and the comparison below is meaningless")
	}

	// The claim.
	if !bytes.Equal(historyBody, visibleBody) {
		t.Errorf("alternate-screen body for the HISTORY-inclusive range is not byte-identical to the visible-only one:\n history-inclusive %q\n    visible-only   %q\nthe suppression must RE-CAPTURE at the visible-only range (only tmux can emit a self-contained -e stream), never slice a capture it already produced", historyBody, visibleBody)
	}
	// And the colour that a slice loses, stated directly rather than only
	// through the comparison above.
	if !bytes.Contains(historyBody, []byte("\x1b[41m")) {
		t.Errorf("alternate-screen history-range body carries no ESC[41m at all, so the alternate screen's red bar would seed with a DEFAULT background: %q", historyBody)
	}

	// Control (1): there really was history above the alternate screen.
	raw, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane (alternate screen, history-inclusive): %v", err)
	}
	if got := captureRowCount(raw); got <= paneHeight {
		t.Fatalf("raw history-inclusive capture returned %d rows (<= pane_height %d): tmux is not prepending the primary screen's history here, so this fixture exercises no suppression at all", got, paneHeight)
	}

	// Control (2): the retired implementation really does differ, and
	// differs by a missing SGR introducer on row 0.
	naive := naiveLastRowsSlice(raw, paneHeight)
	if bytes.Equal(naive, visibleBody) {
		t.Fatalf("a naive Go slice of the raw history-inclusive capture is byte-identical to the visible-only capture for this fixture, so the fixture could not have caught the bug this test exists for: the history row above the alternate screen must leave a NON-DEFAULT pen (see paneWithColouredHistoryAboveAlternateScreen)")
	}
	visibleFirstRow := strings.SplitN(string(visibleBody), "\n", 2)[0]
	naiveFirstRow := strings.SplitN(string(naive), "\n", 2)[0]
	if !strings.HasPrefix(visibleFirstRow, "\x1b") {
		t.Fatalf("the visible-only capture's row 0 does not begin with an escape sequence (%q), so the missing-introducer control below cannot be stated", visibleFirstRow)
	}
	if strings.HasPrefix(naiveFirstRow, "\x1b") {
		t.Fatalf("the naive slice's row 0 DOES begin with an escape sequence (%q); tmux emitted an introducer at the history -> alternate-screen boundary after all, so this fixture no longer demonstrates the inheritance the bug turned on", naiveFirstRow)
	}
}

// sgrAndOSCPattern matches the two escape shapes `capture-pane -e` can
// emit around cell content -- a CSI sequence (SGR) and an OSC string
// (hyperlinks) -- the same two classes internal/interactive's own
// gridContains strips before searching a rendered row.
var sgrAndOSCPattern = regexp.MustCompile(`\x1b\[[0-9:;]*[A-Za-z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// widestCaptureRow returns the widest DISPLAY width of any row in a
// capture body, and that row's text, with escape sequences stripped
// first. Display width, not byte length: a body captured with -e carries
// SGR bytes that occupy no columns, and real pane content can carry
// multi-byte and double-width runes.
func widestCaptureRow(body []byte) (width int, row string) {
	for _, raw := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		plain := sgrAndOSCPattern.ReplaceAllString(raw, "")
		if w := runewidth.StringWidth(plain); w > width {
			width, row = w, plain
		}
	}
	return width, row
}

// TestHistoryInclusiveCaptureRowsNeverExceedTheCurrentPaneWidth settles,
// by measurement, the one question that could invalidate issue #29's
// whole design: does tmux REFLOW a pane's existing history when the pane
// is made narrower, or does it hand back rows still laid out at the old,
// wider geometry?
//
// It matters because the entry seed is taken AFTER deck has fitted the
// tmux window to the preview pane's own geometry, which routinely means
// narrowing it. The seed replays each captured row into one grid row and
// relies on that 1:1 mapping; a row wider than the grid would wrap onto
// the next grid row, and every row below it would land one line lower
// than it should -- the same class of misalignment as the trailing-blank
// bug, but unfixable by a trim, because it would need real rewrapping
// logic the design does not have.
//
// The measured answer, pinned here: tmux DOES reflow. History produced at
// 80 columns and then narrowed to 40 comes back as more, shorter rows
// (history_size grew from 33 to 74 in the original measurement), none of
// them wider than the pane's current width. The pre-resize assertion is
// the non-vacuity control: at 80 columns the very same content DOES
// produce rows wider than 40, so a tmux that simply returned unreflowed
// rows would fail the post-resize assertion rather than pass it by
// accident.
func TestHistoryInclusiveCaptureRowsNeverExceedTheCurrentPaneWidth(t *testing.T) {
	const wideWidth, narrowWidth, paneHeight = 80, 40, 10
	socket := geometrySocket("seed-history-reflow")
	cleanup := newBareGeometrySession(t, socket, "s0", wideWidth, paneHeight)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Lines of exactly 76 characters ("hist-mark-" is ten, plus 66 digits):
	// comfortably inside 80 columns, so nothing wraps at the ORIGINAL
	// width and the fixture's own premise is clean, and comfortably wider
	// than 40, so every one of them must be reflowed for the post-resize
	// assertion to hold.
	cmd := "i=0; while [ $i -lt 40 ]; do i=$((i+1)); printf '" + historyMarker + "-%066d\\n' $i; done"
	runTmux(t, socket, "send-keys", "-t", "s0", "-l", "--", cmd)
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	time.Sleep(500 * time.Millisecond)

	paneID := resolveSolePaneID(t, socket, "s0")
	wideHistory, _ := paneHistoryAndHeight(t, socket, "s0")
	if wideHistory <= 0 {
		t.Fatalf("fixture produced history_size %d at %d columns; there is no history to reflow", wideHistory, wideWidth)
	}
	wideBody, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane at %d columns: %v", wideWidth, err)
	}
	if wide, row := widestCaptureRow(wideBody); wide <= narrowWidth {
		t.Fatalf("widest captured row at %d columns is %d columns (%q); it must exceed the narrow width %d or the post-resize assertion proves nothing", wideWidth, wide, row, narrowWidth)
	}

	// Narrow the pane, the way fitting the tmux window to a preview panel
	// does before the entry seed is taken.
	runTmux(t, socket, "resize-window", "-t", "s0", "-x", strconv.Itoa(narrowWidth), "-y", strconv.Itoa(paneHeight))
	time.Sleep(500 * time.Millisecond)
	gotWidth := runTmux(t, socket, "display-message", "-p", "-t", "s0", "#{pane_width}")
	if strings.TrimSpace(gotWidth) != strconv.Itoa(narrowWidth) {
		t.Fatalf("pane_width after resize = %q, want %d; the resize did not take", gotWidth, narrowWidth)
	}
	narrowHistory, narrowHeight := paneHistoryAndHeight(t, socket, "s0")

	narrowBody, err := client.CapturePane(ctx, paneID, SeedCaptureOptionsWithHistory(2000))
	if err != nil {
		t.Fatalf("CapturePane at %d columns: %v", narrowWidth, err)
	}
	if widest, row := widestCaptureRow(narrowBody); widest > narrowWidth {
		t.Fatalf("after narrowing to %d columns a history-inclusive capture still returned a %d-column row (%q): tmux is NOT reflowing history, so the seed's 1:1 row mapping is invalid and the grid would need rewrapping logic this design does not have", narrowWidth, widest, row)
	}
	// The reflow's own signature: the same content occupies MORE rows at
	// the narrower width, and the capture's row count still matches the
	// clamp arithmetic the seed relies on.
	if narrowHistory <= wideHistory {
		t.Errorf("history_size went %d -> %d across a narrowing resize; 76-column content reflowed into 40 columns must occupy strictly more rows", wideHistory, narrowHistory)
	}
	if got, want := captureRowCount(narrowBody), narrowHistory+narrowHeight; got != want {
		t.Errorf("post-reflow history-inclusive capture returned %d rows, want %d (history_size %d + pane_height %d)", got, want, narrowHistory, narrowHeight)
	}
	if !bytes.Contains(narrowBody, []byte(historyMarker+"-0")) {
		t.Errorf("post-reflow capture lost the fixture's content entirely: %q", narrowBody)
	}
}
