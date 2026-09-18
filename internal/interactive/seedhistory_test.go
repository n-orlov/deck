// seedhistory_test.go proves issue #29 end to end on this side of the
// tmux seam: the ENTRY seed pulls the pane's tmux-side scrollback into the
// grid (so Shift+PgUp in a freshly entered interactive preview reaches
// something at all), it does so without shifting the pane's live screen
// by even one row, the periodic reseed loops deliberately stay on the
// cheap visible-only range -- structurally, not just consequentially (see
// TestNoNonTestFileInThisPackageAsksForTheHistoryRange) -- an
// alternate-screen pane is seeded with no history whatsoever, the
// degradation to no-history fires for a busy pane and for nothing else,
// and the range the ENTRY seed asks for depends on the transport that is
// about to consume it (EntrySeedHistoryLines).
package interactive

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// seedHistoryMarker is the text the real-pane fixtures below print, one
// numbered line at a time. "%04d" formatting matters for the same reason
// it does in internal/tmux/paneseed_history_test.go: the shell ECHOES the
// command that produces these lines, so the pane's visible screen always
// contains the literal "seed-hist-%04d" from that echo, and only a
// FORMATTED instance ("seed-hist-0001") is evidence of real produced
// output.
const seedHistoryMarker = "seed-hist"

func seedHistoryLine(n int) string {
	return fmt.Sprintf("%s-%04d", seedHistoryMarker, n)
}

// runPaneCommand types one shell command into target as literal
// keystrokes and waits for tmux to have processed the result, the same
// technique grid_test.go's sendLiteralLine and seed_test.go's
// runShellPrintf use.
func runPaneCommand(t *testing.T, socket, target, cmd string) {
	t.Helper()
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "-l", "--", cmd).CombinedOutput(); err != nil {
		t.Fatalf("send-keys -l -- %q: %v: %s", cmd, err, out)
	}
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("send-keys Enter: %v: %s", err, out)
	}
	time.Sleep(600 * time.Millisecond)
}

// fillPaneHistory makes target produce `lines` numbered lines -- enough of
// them to scroll well off a short pane and become real tmux scrollback.
// The command is a POSIX `while` loop rather than `seq`, so it needs
// nothing beyond the /bin/sh tmux falls back to inside ci/run.sh's
// container.
//
// thenBlock parks the pane in `cat > /dev/null` afterwards. Quiescence is
// not a nicety for the tests that ask for it: they take TWO captures of
// the same pane and compare them byte for byte, and they rely on
// CapturePaneSeedAtomic's before/after probes agreeing on the first
// attempt. A test that needs to send a SECOND command to the pane (the
// alternate-screen switch) must pass false, since a pane sitting in `cat`
// would swallow that command as stdin instead of running it.
func fillPaneHistory(t *testing.T, socket, target string, lines int, thenBlock bool) {
	t.Helper()
	cmd := "i=0; while [ $i -lt " + strconv.Itoa(lines) + " ]; do i=$((i+1)); printf '" + seedHistoryMarker + "-%04d\\n' $i; done"
	if thenBlock {
		cmd += "; cat > /dev/null"
	}
	runPaneCommand(t, socket, target, cmd)
}

// gridRows returns the grid's VISIBLE rows as plain text, one string per
// row, with the styling escapes Render() interleaves stripped -- the same
// treatment gridContains applies before searching, but keeping the rows
// separate so a test can assert on a row's POSITION rather than merely on
// the presence of a needle somewhere on screen.
func gridRows(g *Grid) []string {
	rows := strings.Split(g.Render(), "\n")
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = strings.TrimRight(ansiEscapeRe.ReplaceAllString(row, ""), " ")
	}
	return out
}

// TestBuildSeedKeepsTrailingBlankRowsBottomAnchored is the regression test
// for the fatal down-shift issue #29's history pull would otherwise have
// introduced, and the single most important test in this file.
//
// BuildSeed used to trim its body with bytes.TrimRight(body, "\n"), which
// removes not just capture-pane's own terminator but the WHOLE trailing
// run of bare empty lines that `-N` deliberately preserves as real blank
// rows. With a visible-only body that was harmless -- a body exactly as
// tall as the grid paints from the TOP row down either way -- but a
// history-inclusive body is taller than the grid, so its rows are
// BOTTOM-anchored: the last body row lands on the grid's last row and
// everything above it is pushed up into scrollback. Dropping K trailing
// blank rows therefore shifts the entire picture DOWN by K rows and
// pushes K rows of the pane's LIVE screen up into scrollback, where a
// viewer never sees them.
//
// No pre-existing test could catch it: gridContains only asks whether a
// needle is somewhere on the rendered screen, and a down-shifted body
// still leaves its content visible -- just in the wrong place. So this
// asserts on ROW POSITION explicitly, plus on the resulting scrollback
// length, which the shift also gets wrong (by exactly the same K).
func TestBuildSeedKeepsTrailingBlankRowsBottomAnchored(t *testing.T) {
	const width, height = 20, 8
	// A body TALLER than the grid, the shape a history-inclusive capture
	// always has: historyRows of it must end up in the scrollback.
	const historyRows, blankTail = 5, 3
	const bodyRows = height + historyRows

	rows := make([]string, bodyRows)
	for i := range rows {
		if i >= bodyRows-blankTail {
			// A bare empty line: exactly what capture-pane -N emits for a
			// blank, default-styled row, and exactly what TrimRight ate.
			rows[i] = ""
			continue
		}
		rows[i] = fmt.Sprintf("body-row-%02d", i)
	}
	// The body's own shape, as BOTH producers now deliver it (see
	// internal/tmux's TestCapturePaneSeedAtomicMatchesSeparateStateAndBodyReads):
	// R rows joined by R-1 newlines, plus capture-pane's own terminator.
	body := []byte(strings.Join(rows, "\n") + "\n")

	state := tmux.PaneSeedState{
		CursorFlag:        true,
		WrapFlag:          true,
		ScrollRegionLower: height - 1,
	}
	g := newGrid(width, height)
	if _, err := g.Write(BuildSeed(state, body)); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	// 1. Bottom anchoring: the body's LAST row is the pane's last row, so
	// it must land on the grid's last row -- which, with blankTail blank
	// rows at the end, means the last CONTENT row lands blankTail rows
	// ABOVE the bottom, not on it.
	lastContentBody := bodyRows - blankTail - 1
	wantRow := height - 1 - blankTail
	got := gridRows(g)
	if len(got) != height {
		t.Fatalf("rendered %d rows, want the grid's own height %d", len(got), height)
	}
	if want := fmt.Sprintf("body-row-%02d", lastContentBody); got[wantRow] != want {
		t.Errorf("grid row %d = %q, want %q (the last CONTENT row must sit %d rows above the bottom, one per preserved trailing blank row)", wantRow, got[wantRow], want, blankTail)
	}
	for y := wantRow + 1; y < height; y++ {
		if got[y] != "" {
			t.Errorf("grid row %d = %q, want it blank: the body's %d trailing blank rows must still be on screen, holding the content up", y, got[y], blankTail)
		}
	}

	// 2. The scrollback length the shift also gets wrong. Exactly
	// historyRows rows scrolled off the top; a K-row down-shift would
	// leave historyRows-K, silently swallowing K rows of the live screen.
	if sb := g.ScrollbackLen(); sb != historyRows {
		t.Errorf("ScrollbackLen() = %d, want %d (bodyRows %d - height %d); a smaller value means trailing blank rows were dropped and the live screen was pushed into scrollback", sb, historyRows, bodyRows, height)
	}

	// 3. And the first row still on screen is the one the arithmetic
	// predicts, so the anchoring is proven at both edges rather than only
	// at the bottom.
	if want := fmt.Sprintf("body-row-%02d", historyRows); got[0] != want {
		t.Errorf("grid row 0 = %q, want %q (body row historyRows, i.e. the first row that did NOT scroll off)", got[0], want)
	}
}

// TestCaptureSeedWithHistoryFillsScrollbackWithoutMovingTheLiveScreen is
// issue #29's headline test: the symptom the issue reports is that
// entering interactive preview on a session that has been running a while
// gives a grid whose 2000-line scrollback is EMPTY, so Shift+PgUp and
// wheel-up reach nothing at all even though tmux still holds every one of
// those lines.
//
// Three assertions, and the third is the one that keeps the fix honest:
//
//	(a) the scrollback is non-empty -- it is 0 today, for every pane, no
//	    matter how much the pane printed before entry;
//	(b) RenderRows at a large offset (exactly what internal/tui's
//	    Shift+PgUp handler drives) returns lines printed LONG before the
//	    seed was taken, not merely the rows that were on screen;
//	(c) the grid's VISIBLE rows are byte-identical to what a visible-only
//	    CaptureSeed of the same pane produces -- i.e. pulling history did
//	    not shift the live screen by even one row. (b) without (c) would be
//	    a regression dressed as a feature.
func TestCaptureSeedWithHistoryFillsScrollbackWithoutMovingTheLiveScreen(t *testing.T) {
	const width, height = 40, 10
	socket := interactiveSocket("seed-history-scrollback")
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	// Six times the pane's height, so the great majority of what the pane
	// printed is in tmux's history and nowhere on its screen.
	fillPaneHistory(t, socket, "s0", height*6, true)
	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	withHistory, err := CaptureSeedWithHistory(ctx, client, paneID, ScrollbackMaxLines)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory: %v", err)
	}
	historyGrid := newGrid(width, height)
	if _, err := historyGrid.Write(withHistory); err != nil {
		t.Fatalf("write history-inclusive seed: %v", err)
	}

	// (a) the scrollback is populated at all.
	sbLen := historyGrid.ScrollbackLen()
	if sbLen <= 0 {
		t.Fatalf("ScrollbackLen() = %d after a history-inclusive entry seed, want > 0; this is exactly issue #29's symptom (Shift+PgUp reaches nothing)", sbLen)
	}
	if sbLen > ScrollbackMaxLines {
		t.Errorf("ScrollbackLen() = %d, want no more than the grid's own bound %d", sbLen, ScrollbackMaxLines)
	}

	// (b) the OLDEST rows the grid holds -- what RenderRows returns at the
	// maximum offset, since offset is clamped to the scrollback length and
	// offset == sbLen puts absolute row 0 at the top of the view -- are
	// early lines from before the seed. A SET of early markers is accepted
	// rather than one exact line number, because how many rows the shell's
	// own echoed command occupies depends on the shell's prompt.
	session := &Session{grid: historyGrid}
	oldest, usedOffset := session.RenderRows(sbLen, height)
	if usedOffset != sbLen {
		t.Fatalf("RenderRows(%d, %d) clamped the offset to %d; the scrollback is shorter than ScrollbackLen() claims", sbLen, height, usedOffset)
	}
	oldestText := strings.Join(oldest, "\n")
	foundEarly := ""
	for n := 1; n <= 5; n++ {
		if strings.Contains(oldestText, seedHistoryLine(n)) {
			foundEarly = seedHistoryLine(n)
			break
		}
	}
	if foundEarly == "" {
		t.Errorf("RenderRows at the maximum offset (%d) contains none of the first five lines the pane ever printed; the grid's scrollback does not reach back to before the seed:\n%s", sbLen, oldestText)
	}

	// (b, control) those early lines are NOT on the live screen, so the
	// assertion above really did require the scrollback and could not have
	// been satisfied by the visible rows alone.
	live, _ := session.RenderRows(0, height)
	liveText := strings.Join(live, "\n")
	for n := 1; n <= 5; n++ {
		if strings.Contains(liveText, seedHistoryLine(n)) {
			t.Fatalf("line %q is on the pane's LIVE screen, so the fixture did not push it into history and assertion (b) is vacuous", seedHistoryLine(n))
		}
	}

	// (c) the live screen is exactly where a visible-only seed would have
	// put it. This is what proves the history rows went into the
	// scrollback ABOVE the live screen rather than displacing it -- the
	// bottom-anchoring property BuildSeed's trim rule delivers.
	visibleOnly, err := CaptureSeed(ctx, client, paneID)
	if err != nil {
		t.Fatalf("CaptureSeed: %v", err)
	}
	visibleGrid := newGrid(width, height)
	if _, err := visibleGrid.Write(visibleOnly); err != nil {
		t.Fatalf("write visible-only seed: %v", err)
	}
	wantRows, gotRows := gridRows(visibleGrid), gridRows(historyGrid)
	if len(wantRows) != len(gotRows) {
		t.Fatalf("history-inclusive seed rendered %d rows, visible-only seed %d", len(gotRows), len(wantRows))
	}
	for y := range wantRows {
		if gotRows[y] != wantRows[y] {
			t.Errorf("visible row %d differs between the two seeds:\n history-inclusive %q\n    visible-only   %q\nthe history pull shifted the live screen", y, gotRows[y], wantRows[y])
		}
	}
}

// TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty pins the DELIBERATE
// other half of issue #29's design: CaptureSeed -- the seed the two
// PERIODIC reseed loops use, captureLoop under TransportCapture and
// fallbackLoop after the pipe is displaced -- still captures the visible
// screen only, so a grid it seeds has an empty scrollback and scrolling
// back under TransportCapture still reaches nothing.
//
// That is a cost decision, not an oversight. Both loops rebuild the grid
// WHOLESALE every 200ms (capturePollInterval,
// pipeDisplacedFallbackInterval), and a history-inclusive reseed at that
// cadence was measured in this repository at ~37ms of CPU and ~21.5MB of
// garbage per tick for a 2040-row seed, against ~0.7ms for today's 40-row
// one -- roughly a fifth of a core at this test's 120x40-class geometry
// and over half a core at the operator's real 166x66 panes, plus ~107MB/s
// of allocation churn. It is also not a regression: the interactive
// scrollback was ALREADY always empty under TransportCapture, because a
// body exactly as tall as the grid never scrolls a row off it.
//
// SCOPE, so this test is not mistaken for more than it is: it calls
// CaptureSeed DIRECTLY, so it pins that the visible-only range HAS the
// property claimed above and nothing whatsoever about which range
// captureLoop and fallbackLoop actually ask for. Pointing both of them at
// CaptureSeedWithHistory leaves this test, and every other test in
// internal/interactive, internal/tui and features, entirely green. The
// guard that catches that is
// TestNoNonTestFileInThisPackageAsksForTheHistoryRange, and it is the one
// to read before changing either loop -- including for the re-measurement
// requirement, which applies here too: the failure mode is a preview
// panel that burns CPU continuously rather than one that looks wrong,
// which is why it needs a test to say so out loud.
func TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty(t *testing.T) {
	const width, height = 40, 10
	socket := interactiveSocket("seed-visible-only-empty-sb")
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	fillPaneHistory(t, socket, "s0", height*6, true)
	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	seed, err := CaptureSeed(ctx, client, paneID)
	if err != nil {
		t.Fatalf("CaptureSeed: %v", err)
	}
	g := newGrid(width, height)
	if _, err := g.Write(seed); err != nil {
		t.Fatalf("write visible-only seed: %v", err)
	}
	if sb := g.ScrollbackLen(); sb != 0 {
		t.Errorf("ScrollbackLen() = %d after a VISIBLE-ONLY seed, want 0: the periodic reseed loops must stay on the cheap range (see this test's doc for the measured per-tick cost)", sb)
	}

	// Non-vacuity: the very same pane, seeded with history, DOES fill the
	// scrollback -- so the zero above is a property of the range, not of
	// the fixture having no history to find.
	withHistory, err := CaptureSeedWithHistory(ctx, client, paneID, ScrollbackMaxLines)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory: %v", err)
	}
	historyGrid := newGrid(width, height)
	if _, err := historyGrid.Write(withHistory); err != nil {
		t.Fatalf("write history-inclusive seed: %v", err)
	}
	if sb := historyGrid.ScrollbackLen(); sb <= 0 {
		t.Fatalf("the same pane seeded WITH history has ScrollbackLen() = %d; the fixture has no history, so this test proves nothing about the range", sb)
	}
}

// TestCaptureSeedWithHistoryOnAlternateScreenPaneHasNoHistoryAtAll is the
// alt-screen exclusion (issue #29) seen from this side of the seam: a pane
// parked on its alternate screen -- an editor, a pager, a full-screen
// agent TUI -- must be seeded with its alternate screen and NOTHING else.
//
// tmux keeps the PRIMARY screen's history untouched while the alternate
// screen is up and will happily prepend it to a `-S -2000` capture
// (internal/tmux's TestCapturePaneSeedAtomicSuppressesAlternateScreenHistory
// measures that directly). Those rows are not scrollback a viewer could
// ever reach -- an alternate screen has none, not even under a real `tmux
// attach` -- and on this side they would be worse than useless: the seed
// switches the grid to its alternate buffer FIRST (BuildSeed step 1), and
// a grid's alternate buffer has its own scrollback that vt does not bound
// for deck and that neither RenderRows nor ScrollbackLen reads. Rows
// landing there would be invisible AND unbounded.
func TestCaptureSeedWithHistoryOnAlternateScreenPaneHasNoHistoryAtAll(t *testing.T) {
	const width, height = 40, 10
	socket := interactiveSocket("seed-history-altscreen")
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	// Real history first, then the alternate-screen switch with some
	// content of its own, then a blocking `cat` so the shell never
	// redraws a prompt and switches straight back.
	fillPaneHistory(t, socket, "s0", height*6, false)
	runPaneCommand(t, socket, "s0", `printf "\033[?1049h\033[HALT-SCREEN-ONLY-CONTENT"; cat > /dev/null`)
	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	state, err := client.PaneSeedState(ctx, paneID)
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}
	if !state.AlternateOn {
		t.Fatalf("AlternateOn = false after ESC[?1049h; fixture assumption violated (state %+v)", state)
	}

	seed, err := CaptureSeedWithHistory(ctx, client, paneID, ScrollbackMaxLines)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory: %v", err)
	}
	// The strongest form of the claim: the pre-launch text is not even
	// present in the seed BYTES, so no amount of later mode switching
	// inside the grid could surface it.
	if strings.Contains(string(seed), seedHistoryLine(1)) {
		t.Errorf("the alternate-screen pane's seed contains %q, a line the alternate screen never showed", seedHistoryLine(1))
	}

	g := newGrid(width, height)
	if _, err := g.Write(seed); err != nil {
		t.Fatalf("write alternate-screen seed: %v", err)
	}
	if sb := g.ScrollbackLen(); sb != 0 {
		t.Errorf("ScrollbackLen() = %d for an alternate-screen pane, want 0", sb)
	}
	for n := 1; n <= 5; n++ {
		if gridContains(g, seedHistoryLine(n)) {
			t.Errorf("the seeded grid shows %q, which was primary-screen history the alternate screen never showed", seedHistoryLine(n))
		}
	}
	// Non-vacuity: the alternate screen's OWN content did arrive, so the
	// emptiness above is suppression of history and not a failed seed.
	if !gridContains(g, "ALT-SCREEN-ONLY-CONTENT") {
		t.Errorf("the seeded grid does not show the alternate screen's own content; the seed itself failed, so the assertions above prove nothing:\n%q", g.Render())
	}
}

// fakeSeedProbeScript writes a fake tmux binary that answers any
// invocation with the chained three-line-boundaries shape
// CapturePaneSeedAtomic parses, and chooses its before/after probe values
// FROM THE REQUESTED RANGE: a history-inclusive capture ("-S -2000"
// somewhere in argv) always gets disagreeing probes, so
// CapturePaneSeedAtomic exhausts its retries and fails, while a
// visible-only capture ("-S 0") gets agreeing probes if and only if
// visibleOnlyAgrees.
//
// That is exactly the situation issue #29's degradation path exists for,
// and -- since the degradation now fires for this failure and no other --
// the only situation that can drive it at all: disagreeing probes are
// what CapturePaneSeedAtomic reports as
// tmux.PaneSeedProbesNeverAgreedError. A pane chatty enough to keep
// #{history_size} moving through twenty attempts is real -- the wider
// capture takes longer, and history_size is not even monotonic, since the
// shell's `clear` zeroes it -- and the caller of the entry seed turns any
// error into a refusal to enter interactive mode at all.
//
// rangeLogPath accumulates one line per invocation naming which range it
// was asked for, so a test can assert the fallback was a visible-only
// capture rather than merely a second attempt at the same thing.
func fakeSeedProbeScript(t *testing.T, visibleOnlyAgrees bool) (binary, rangeLogPath string) {
	t.Helper()
	dir := t.TempDir()
	rangeLogPath = filepath.Join(dir, "ranges")
	binary = filepath.Join(dir, "tmux")
	// Sixteen state fields matching paneSeedStateFieldCount: alternate
	// screen off, cursor visible, wrap on, a 10-row scroll region. Four
	// discriminator fields follow on the same line (issue #29 made
	// #{alternate_on} the fourth).
	const stateSixteen = "0|0|0|1|0|0|0|0|0|0|0|0|1|0|0|9"
	visibleAfter := "5|40|10|0"
	if !visibleOnlyAgrees {
		visibleAfter = "6|40|10|0"
	}
	script := `#!/bin/sh
case "$*" in
  *"-S -2000"*)
    echo history-inclusive >> "` + rangeLogPath + `"
    before="5|40|10|0"
    after="6|40|10|0"
    ;;
  *)
    echo visible-only >> "` + rangeLogPath + `"
    before="5|40|10|0"
    after="` + visibleAfter + `"
    ;;
esac
printf '%s|%s\n' "` + stateSixteen + `" "$before"
printf 'FALLBACK-SEED-BODY-ROW\n'
printf '%s\n' "$after"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, rangeLogPath
}

// readRangeLog returns the per-invocation range log fakeSeedProbeScript
// accumulates, oldest first.
func readRangeLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read range log: %v", err)
	}
	return strings.Fields(strings.TrimSpace(string(data)))
}

// TestCaptureSeedWithHistoryDegradesToNoHistoryRatherThanFailing is issue
// #29's graceful-degradation requirement. The history-inclusive capture is
// a WIDER capture than deck used to take, so it takes longer and widens
// CapturePaneSeedAtomic's own before/after race window; a busy enough pane
// can genuinely exhaust maxPaneSeedAtomicAttempts. Since the entry seed's
// caller turns any error into "Cannot enter interactive mode: ...",
// failing there would mean a busy pane could not be previewed
// interactively AT ALL -- a far worse regression than previewing it
// without history. So the failure is retried once at historyLines = 0 and
// that seed is returned.
//
// The final assertion is what keeps the fixture honest now that the
// degradation is CONDITIONAL on the failure's class: it checks that the
// error this fake produces really is
// tmux.PaneSeedProbesNeverAgreedError. Without it, a fake that started
// failing some other way (a parse error, a non-zero exit) would make this
// test go red for a reason that looks like a regression in the
// degradation and is not; and its twin,
// TestCaptureSeedWithHistoryDoesNotRetryAFailureNoNarrowerRangeCanFix,
// would have nothing to contrast with.
func TestCaptureSeedWithHistoryDegradesToNoHistoryRatherThanFailing(t *testing.T) {
	binary, rangeLog := fakeSeedProbeScript(t, true)
	client := tmux.Client{Binary: binary, Socket: "deck-seed-degrade", Timeout: 5 * time.Second}

	seed, err := CaptureSeedWithHistory(context.Background(), client, "%0", ScrollbackMaxLines)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory against a pane whose history-inclusive probes never agree: want the visible-only fallback seed, got error: %v", err)
	}
	if !strings.Contains(string(seed), "FALLBACK-SEED-BODY-ROW") {
		t.Errorf("returned seed does not contain the fixture's body row; it is not a real seed: %q", seed)
	}

	ranges := readRangeLog(t, rangeLog)
	if len(ranges) == 0 {
		t.Fatalf("fake tmux was never invoked")
	}
	if last := ranges[len(ranges)-1]; last != "visible-only" {
		t.Errorf("the LAST capture attempted was %q, want %q: the degradation must be a narrower range, not another attempt at the same one", last, "visible-only")
	}
	if first := ranges[0]; first != "history-inclusive" {
		t.Errorf("the FIRST capture attempted was %q, want %q: history must be attempted before it is given up on", first, "history-inclusive")
	}
	// The visible-only fallback is ONE attempt at CaptureSeedWithHistory's
	// level (CapturePaneSeedAtomic's own retry loop is not re-entered by
	// it, and it agrees on its first try here), so exactly one line of the
	// log may name it.
	visibleOnly := 0
	for _, r := range ranges {
		if r == "visible-only" {
			visibleOnly++
		}
	}
	if visibleOnly != 1 {
		t.Errorf("the visible-only range was captured %d times, want exactly 1 (the fallback is a single retry, not a second retry loop)", visibleOnly)
	}

	// Fixture non-vacuity, deliberately last so its own invocations cannot
	// pollute the range log read above: the failure this fake induces is
	// the ONE class the degradation is allowed to act on.
	_, _, directErr := client.CapturePaneSeedAtomic(context.Background(), "%0", tmux.SeedCaptureOptionsWithHistory(ScrollbackMaxLines))
	var neverAgreed *tmux.PaneSeedProbesNeverAgreedError
	if !errors.As(directErr, &neverAgreed) {
		t.Fatalf("the fake's history-inclusive failure is %v, which is NOT tmux.PaneSeedProbesNeverAgreedError; the degradation only fires for that class, so this fixture no longer exercises it", directErr)
	}
}

// fakeAlternateScreenRecaptureNeverAgreesScript is a tmux whose
// history-inclusive probes AGREE while reporting the pane is on its
// alternate screen, and whose visible-only probes never agree.
//
// That combination is what reaches CapturePaneSeedAtomic's alternate-screen
// re-capture (issue #29: tmux returns stale pre-launch history ABOVE an
// alternate screen, so a history-inclusive request for such a pane is
// answered by re-capturing visible-only, which only that function can do
// because #{alternate_on} arrives from the same chained invocation as the
// body) and then makes that INNER capture run out of attempts -- so the
// error CaptureSeedWithHistory sees is a probes-never-agreed failure whose
// range was ALREADY visible-only.
func fakeAlternateScreenRecaptureNeverAgreesScript(t *testing.T) (binary, rangeLogPath string) {
	t.Helper()
	dir := t.TempDir()
	rangeLogPath = filepath.Join(dir, "ranges")
	binary = filepath.Join(dir, "tmux")
	// Sixteen state fields matching paneSeedStateFieldCount, with the
	// FIRST (#{alternate_on}) set: this pane is on its alternate screen.
	const stateAlternateOn = "1|0|0|1|0|0|0|0|0|0|0|0|1|0|0|9"
	script := `#!/bin/sh
case "$*" in
  *"-S -2000"*)
    echo history-inclusive >> "` + rangeLogPath + `"
    before="5|40|10|1"
    after="5|40|10|1"
    ;;
  *)
    echo visible-only >> "` + rangeLogPath + `"
    before="5|40|10|1"
    after="6|40|10|1"
    ;;
esac
printf '%s|%s\n' "` + stateAlternateOn + `" "$before"
printf 'ALT-SCREEN-BODY-ROW\n'
printf '%s\n' "$after"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, rangeLogPath
}

// TestCaptureSeedWithHistoryDoesNotDegradeAFailureAlreadyAtTheVisibleOnlyRange
// pins the one case where the degradation's own premise does not hold.
//
// CaptureSeedWithHistory degrades to the visible-only range because a
// narrower capture is a shorter race window, so a pane too chatty to pair
// over its whole history may still pair over its visible screen. That
// reasoning is only valid when the range that failed was the WIDE one. On
// an alternate-screen pane it is not: CapturePaneSeedAtomic answers a
// history-inclusive request by re-capturing visible-only ITSELF, and it is
// that inner capture which can exhaust maxPaneSeedAtomicAttempts. Retrying
// from out here would then spend a second full round of attempts on a
// BYTE-IDENTICAL request and report "history-inclusive capture failed ...
// and the visible-only retry failed too", naming two ranges when only one
// was ever tried -- doubling the time Enter blocks bubbletea's Update
// goroutine to learn nothing.
//
// The expected attempt count is measured from the fixture rather than
// written as a literal, so this test cannot drift if
// maxPaneSeedAtomicAttempts changes: one pairing loop's worth of
// visible-only invocations is exactly what a single direct
// CapturePaneSeedAtomic call against the same fake produces.
func TestCaptureSeedWithHistoryDoesNotDegradeAFailureAlreadyAtTheVisibleOnlyRange(t *testing.T) {
	ctx := context.Background()
	binary, rangeLog := fakeAlternateScreenRecaptureNeverAgreesScript(t)
	client := tmux.Client{Socket: "deck-test", Binary: binary}

	// One pairing loop's worth of visible-only invocations, measured.
	if _, _, err := client.CapturePaneSeedAtomic(ctx, "%0", tmux.SeedCaptureOptions()); err == nil {
		t.Fatalf("the fake agreed on the visible-only range; this fixture only exercises the case it exists for if that range NEVER pairs")
	}
	onePairingLoop := len(readRangeLog(t, rangeLog))
	if onePairingLoop < 2 {
		t.Fatalf("a single visible-only pairing loop logged %d invocations, want the full retry ladder; the fixture is not disagreeing the way this test needs", onePairingLoop)
	}
	if err := os.Remove(rangeLog); err != nil {
		t.Fatalf("reset range log: %v", err)
	}

	seed, err := CaptureSeedWithHistory(ctx, client, "%0", ScrollbackMaxLines)
	if err == nil {
		t.Fatalf("CaptureSeedWithHistory returned a seed (%d bytes) against a fake whose visible-only range never pairs; want the failure reported", len(seed))
	}

	ranges := readRangeLog(t, rangeLog)
	history, visible := 0, 0
	for _, r := range ranges {
		switch r {
		case "history-inclusive":
			history++
		case "visible-only":
			visible++
		}
	}
	// One history-inclusive invocation (its probes agree immediately and
	// report AlternateOn), then exactly ONE pairing loop of visible-only
	// invocations from inside CapturePaneSeedAtomic's re-capture. A second
	// loop would mean CaptureSeedWithHistory degraded on top of it.
	if history != 1 {
		t.Errorf("the fake logged %d history-inclusive invocations, want 1 (its probes agree on the first try): %v", history, ranges)
	}
	if visible != onePairingLoop {
		t.Errorf("the fake logged %d visible-only invocations, want %d -- exactly ONE pairing loop, the alternate-screen re-capture inside CapturePaneSeedAtomic. %d would be that loop run twice, i.e. CaptureSeedWithHistory degrading to a range that had already just failed, doubling how long Enter blocks the TUI to re-ask a byte-identical question", visible, onePairingLoop, 2*onePairingLoop)
	}
	if strings.Contains(err.Error(), "visible-only retry failed too") {
		t.Errorf("the error reports a two-range degradation (%q), but only the visible-only range was ever attempted -- an operator reading this would look for a history-inclusive capture that never happened", err)
	}
	var neverAgreed *tmux.PaneSeedProbesNeverAgreedError
	if !errors.As(err, &neverAgreed) {
		t.Fatalf("the error is %v, not tmux.PaneSeedProbesNeverAgreedError; the skip is keyed on that type carrying the range it failed on, so this fixture no longer exercises it", err)
	}
	if neverAgreed.Options != tmux.SeedCaptureOptions() {
		t.Errorf("the failed range is %+v, want SeedCaptureOptions() (%+v) -- the skip reads exactly this field to decide there is nothing narrower left to try", neverAgreed.Options, tmux.SeedCaptureOptions())
	}
}

// TestCaptureSeedWithHistoryDoesNotSwallowTheFallbacksOwnFailure is the
// other side of the degradation: degrading is only allowed to hide the
// HISTORY-inclusive failure. If the visible-only retry fails too, the pane
// is genuinely unreadable (gone, or the socket is broken) and the caller
// must hear about it rather than be handed an empty seed that would paint
// a blank interactive preview over a live pane.
func TestCaptureSeedWithHistoryDoesNotSwallowTheFallbacksOwnFailure(t *testing.T) {
	binary, rangeLog := fakeSeedProbeScript(t, false)
	client := tmux.Client{Binary: binary, Socket: "deck-seed-degrade-fail", Timeout: 5 * time.Second}

	seed, err := CaptureSeedWithHistory(context.Background(), client, "%0", ScrollbackMaxLines)
	if err == nil {
		t.Fatalf("CaptureSeedWithHistory with BOTH ranges failing: want an error, got a %d-byte seed", len(seed))
	}
	if seed != nil {
		t.Errorf("CaptureSeedWithHistory returned a %d-byte seed alongside its error; a caller that ignores the error must not be handed a half-seed", len(seed))
	}
	// The message must name both attempts: a report that only mentioned
	// the visible-only retry would hide the fact that history was tried
	// and lost, which is the one clue that the pane was merely too busy.
	if msg := err.Error(); !strings.Contains(msg, "history-inclusive") || !strings.Contains(msg, "visible-only") {
		t.Errorf("error = %v, want it to name BOTH the history-inclusive failure and the visible-only retry", err)
	}
	ranges := readRangeLog(t, rangeLog)
	if len(ranges) < 2 {
		t.Fatalf("fake tmux logged %v, want both ranges to have been attempted", ranges)
	}
}

// TestCaptureSeedWithHistoryAtZeroIsExactlyCaptureSeed pins the degenerate
// argument: historyLines <= 0 must produce the visible-only seed and, in
// particular, must NOT spend a second attempt degrading to a range it is
// already using (there is nothing narrower to degrade to).
func TestCaptureSeedWithHistoryAtZeroIsExactlyCaptureSeed(t *testing.T) {
	const width, height = 40, 10
	socket := interactiveSocket("seed-history-zero")
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	fillPaneHistory(t, socket, "s0", height*6, true)
	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	zero, err := CaptureSeedWithHistory(ctx, client, paneID, 0)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory(0): %v", err)
	}
	visible, err := CaptureSeed(ctx, client, paneID)
	if err != nil {
		t.Fatalf("CaptureSeed: %v", err)
	}
	if string(zero) != string(visible) {
		t.Errorf("CaptureSeedWithHistory(..., 0) = %q, want byte-identical to CaptureSeed's %q", zero, visible)
	}

	// And the failure path: with nothing narrower to fall back to, a
	// failing capture must be reported once, not attempted twice.
	binary, rangeLog := fakeSeedProbeScript(t, false)
	fake := tmux.Client{Binary: binary, Socket: "deck-seed-zero-fail", Timeout: 5 * time.Second}
	if _, err := CaptureSeedWithHistory(context.Background(), fake, "%0", 0); err == nil {
		t.Fatalf("CaptureSeedWithHistory(..., 0) against a permanently disagreeing fake tmux: want error, got nil")
	}
	for _, r := range readRangeLog(t, rangeLog) {
		if r != "visible-only" {
			t.Fatalf("historyLines = 0 attempted the %q range; it must ask for exactly the visible screen", r)
		}
	}
}

// fakeHardFailureTmuxScript writes a fake tmux that logs the range it was
// asked for and then FAILS OUTRIGHT -- a non-zero exit with a message on
// stderr, the shape a dead socket, a vanished pane or a tmux the wrong
// version all produce. It is deliberately not fakeSeedProbeScript's
// disagreeing-probes failure: this is the class CaptureSeedWithHistory
// must NOT degrade past, because a narrower range would fail in exactly
// the same way.
func fakeHardFailureTmuxScript(t *testing.T) (binary, rangeLogPath string) {
	t.Helper()
	dir := t.TempDir()
	rangeLogPath = filepath.Join(dir, "ranges")
	binary = filepath.Join(dir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"-S -2000"*) echo history-inclusive >> "` + rangeLogPath + `" ;;
  *)            echo visible-only      >> "` + rangeLogPath + `" ;;
esac
echo "no server running on /tmp/tmux-0/deck" >&2
exit 1
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, rangeLogPath
}

// fakeHangingTmuxScript writes a fake tmux that logs the range and then
// never answers at all, so the only thing that ends the call is
// tmux.Client's own Timeout. That is the shape of the failure the
// degradation bug was measured on: a tmux server that has stopped
// responding, not one that refuses.
//
// `exec sleep`, not a bare `sleep`, and that detail is what makes the
// fixture work at all. Client.run drives the child through
// exec.CommandContext(...).CombinedOutput(), which kills only the process
// it started and then waits for the output pipes to close -- so a shell
// that FORKS a long sleeper hands the sleeper the same stdout/stderr, the
// shell dies on the deadline, and CombinedOutput blocks on the still-open
// pipes for the sleeper's full duration instead of returning at the
// timeout. Replacing the shell with the sleeper means the process the
// deadline kills is the one holding the pipes.
func fakeHangingTmuxScript(t *testing.T) (binary, rangeLogPath string) {
	t.Helper()
	dir := t.TempDir()
	rangeLogPath = filepath.Join(dir, "ranges")
	binary = filepath.Join(dir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"-S -2000"*) echo history-inclusive >> "` + rangeLogPath + `" ;;
  *)            echo visible-only      >> "` + rangeLogPath + `" ;;
esac
exec sleep 600
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, rangeLogPath
}

// TestCaptureSeedWithHistoryDoesNotRetryAFailureNoNarrowerRangeCanFix is
// the regression the degradation's narrowing installs. The retry used to
// fire on ANY error from the history-inclusive capture, which quietly
// turned every unrelated failure into two of them in a row -- and the
// entry seed runs on bubbletea's blocking Update goroutine with a
// deadline-less context.Background(), so "two in a row" is measured in
// whole seconds of frozen TUI before "Cannot enter interactive mode: ..."
// can render.
//
// A hard failure (non-zero exit: dead socket, vanished pane, wrong tmux)
// is not something a narrower capture can fix, so exactly ONE capture may
// be attempted. The error must also still be reported, and must not be
// dressed up as the degradation's own two-attempt message, which would
// send a reader looking for a busy pane that was never there.
func TestCaptureSeedWithHistoryDoesNotRetryAFailureNoNarrowerRangeCanFix(t *testing.T) {
	binary, rangeLog := fakeHardFailureTmuxScript(t)
	client := tmux.Client{Binary: binary, Socket: "deck-seed-hard-failure", Timeout: 5 * time.Second}

	seed, err := CaptureSeedWithHistory(context.Background(), client, "%0", ScrollbackMaxLines)
	if err == nil {
		t.Fatalf("CaptureSeedWithHistory against a tmux that exits non-zero: want an error, got a %d-byte seed", len(seed))
	}
	if seed != nil {
		t.Errorf("CaptureSeedWithHistory returned a %d-byte seed alongside its error", len(seed))
	}
	var neverAgreed *tmux.PaneSeedProbesNeverAgreedError
	if errors.As(err, &neverAgreed) {
		t.Fatalf("a non-zero tmux exit was classified as tmux.PaneSeedProbesNeverAgreedError (%v); the fixture no longer distinguishes the two failure classes, so this test cannot prove anything about which one is retried", err)
	}
	if strings.Contains(err.Error(), "visible-only retry") {
		t.Errorf("error = %v, want the plain single-attempt report: the two-attempt degradation message claims a busy pane was found and given up on, which is not what happened", err)
	}
	ranges := readRangeLog(t, rangeLog)
	if len(ranges) != 1 {
		t.Errorf("fake tmux was invoked for %d ranges (%v), want exactly 1: degradation exists for a pane that kept moving (tmux.PaneSeedProbesNeverAgreedError) and for nothing else -- a second attempt at a dead socket only doubles the latency of a failure the caller is about to report", len(ranges), ranges)
	}
	if len(ranges) > 0 && ranges[0] != "history-inclusive" {
		t.Errorf("the one attempt made was %q, want %q", ranges[0], "history-inclusive")
	}
}

// TestCaptureSeedWithHistoryDoesNotDoubleTheLatencyOfAHungServer is the
// same regression stated as the cost it actually had, because that is the
// form the operator experiences. Measured before the fix, against a tmux
// that never answers: CaptureSeed took 5.00s (tmux.Client's default
// Timeout) and CaptureSeedWithHistory took 10.01s, exactly twice, and
// production runs on that 5s default -- so pressing Enter at an
// unresponsive tmux server froze the entire TUI for ten seconds instead
// of five.
//
// The timeout here is scaled down so the test is fast, but the shape is
// identical: one Client.Timeout, not two. The bound is deliberately loose
// (below TWICE the timeout rather than close to one) so this fails on the
// doubling and not on a slow or loaded CI host -- a second attempt cannot
// hide under it, since a second attempt costs a whole second timeout.
func TestCaptureSeedWithHistoryDoesNotDoubleTheLatencyOfAHungServer(t *testing.T) {
	const timeout = 400 * time.Millisecond
	binary, rangeLog := fakeHangingTmuxScript(t)
	client := tmux.Client{Binary: binary, Socket: "deck-seed-hung-server", Timeout: timeout}

	start := time.Now()
	if _, err := CaptureSeedWithHistory(context.Background(), client, "%0", ScrollbackMaxLines); err == nil {
		t.Fatalf("CaptureSeedWithHistory against a tmux that never answers: want a timeout error, got nil")
	}
	elapsed := time.Since(start)
	if elapsed >= 2*timeout {
		t.Errorf("CaptureSeedWithHistory took %s against a %s timeout, i.e. it paid for a second attempt: a hung server answers a narrower range no faster, and the entry seed blocks bubbletea's Update goroutine while it waits", elapsed, timeout)
	}
	if ranges := readRangeLog(t, rangeLog); len(ranges) != 1 {
		t.Errorf("fake tmux was invoked for %d ranges (%v), want exactly 1", len(ranges), ranges)
	}
}

// TestNoNonTestFileInThisPackageAsksForTheHistoryRange is the structural
// half of the cost decision TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty
// only asserts the CONSEQUENCE of, and it is the guard that was missing:
// that test calls CaptureSeed directly, so it cannot see which range
// captureLoop and fallbackLoop actually ask for. Verified: pointing both
// grid.go's captureLoop and its fallbackLoop at
// CaptureSeedWithHistory(ctx, client, target, ScrollbackMaxLines) left
// internal/interactive, internal/tui AND features entirely green while
// multiplying the per-tick cost by roughly fifty.
//
// So the rule this holds is a source-level one, the mirror of
// internal/tui's own
// TestInteractiveEntrySeedAsksForTheTransportsOwnHistoryBound (which pins
// that the ENTRY seed does ask for history): NO non-test file in package
// interactive may call CaptureSeedWithHistory at all. The
// history-inclusive range belongs exclusively to the one-off entry seed,
// which internal/tui owns; everything in this package that reseeds does
// so on a 200ms ticker and must stay on CaptureSeed's visible-only range.
//
// The rule covers the RANGE, not just the wrapper that requests it.
// Forbidding only the CaptureSeedWithHistory call would leave the cheap
// bypass open: a reseed loop that inlined
// CapturePaneSeedAtomic(..., tmux.SeedCaptureOptionsWithHistory(
// ScrollbackMaxLines)) plus BuildSeed pays exactly the same per-tick bill
// while satisfying a wrapper-name check, and for fallbackLoop nothing
// else in the repository would catch it. So
// tmux.SeedCaptureOptionsWithHistory is forbidden here too: the only
// legitimate way for this package's non-test source to name it is not to.
//
// THE COST, so that nobody has to go looking for it before deciding this
// guard is in the way: both loops rebuild the grid WHOLESALE every 200ms
// (capturePollInterval, pipeDisplacedFallbackInterval), and a
// history-inclusive reseed at that cadence was measured in this
// repository at ~37ms of CPU and ~21.5MB of garbage per tick for a
// 2040-row seed, against ~0.7ms for today's 40-row one -- about 19% of a
// core at a 120x40 pane, about 53% of a core at the operator's real
// 166x66 panes, plus ~107MB/s of allocation churn, continuously, for as
// long as a preview panel is open. Changing this therefore requires
// RE-MEASURING that per-tick cost on the target geometry, not editing
// this guard: the failure mode is a preview that burns a core forever,
// which looks perfectly correct on screen and which no behavioural test
// in this package would report.
//
// The check is an AST walk rather than a regexp because the name
// unavoidably appears in this package's non-test source already -- as the
// function's own declaration in seed.go and throughout the doc comments
// that explain why the loops avoid it -- and only a real CALL is a
// violation.
//
// Demonstrate-then-revert: temporarily change captureLoop's
// `CaptureSeed(ctx, client, target)` to
// `CaptureSeedWithHistory(ctx, client, target, ScrollbackMaxLines)` and
// rerun -- this goes red, naming grid.go and the line -- then restore it.
func TestNoNonTestFileInThisPackageAsksForTheHistoryRange(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/interactive): %v", err)
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			// CaptureSeedWithHistory's OWN body is the one sanctioned door
			// to the wide range: it is the wrapper internal/tui calls, so
			// of course it asks tmux for the history-inclusive options.
			// Skipping its subtree is what makes the rule "nothing in this
			// package REACHES the wide range except through the one
			// function that exists to offer it", rather than the weaker
			// "nothing calls the wrapper" -- a reseed loop that inlined
			// CapturePaneSeedAtomic + SeedCaptureOptionsWithHistory +
			// BuildSeed pays the identical per-tick bill, and outside this
			// one body there is no honest reason to name those options.
			if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "CaptureSeedWithHistory" {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var called string
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				called = fun.Name
			case *ast.SelectorExpr:
				called = fun.Sel.Name
			}
			if called == "CaptureSeedWithHistory" || called == "SeedCaptureOptionsWithHistory" {
				t.Errorf("%s:%d: calls %s -- the history-inclusive range belongs to internal/tui's ONE-OFF entry seed only, and that includes reaching it directly through tmux.SeedCaptureOptionsWithHistory rather than through the CaptureSeedWithHistory wrapper. This package's reseeds run on a 200ms ticker, where that range was measured at ~37ms and ~21.5MB per tick (~19%% of a core at 120x40, ~53%% at 166x66) against ~0.7ms for CaptureSeed's visible-only range. Re-measure the per-tick cost at the target geometry before changing this; do not just delete the guard", name, fset.Position(call.Pos()).Line, called)
			}
			return true
		})
	}
}

// TestEntrySeedHistoryLinesIsZeroUnderTheCaptureTransport pins the
// transport-conditional answer internal/tui's entry seed closure asks for
// (EntrySeedHistoryLines), which is the whole of the fix for "the capture
// transport pays for history it throws away 200ms later".
//
// Both directions are asserted, and the constant is compared rather than
// a literal so the pipe answer cannot drift away from what the grid can
// actually hold. The TransportCapture answer is additionally checked
// through tmux.SeedCaptureOptionsWithHistory, because "0" is only the
// right answer if it really does degenerate to the exact visible-only
// range the pre-issue-#29 entry seed used -- a "-0" start line, say, would
// satisfy the integer comparison and ask tmux for something else entirely.
func TestEntrySeedHistoryLinesIsZeroUnderTheCaptureTransport(t *testing.T) {
	if got := EntrySeedHistoryLines(TransportPipe); got != ScrollbackMaxLines {
		t.Errorf("EntrySeedHistoryLines(TransportPipe) = %d, want ScrollbackMaxLines (%d): under the pipe transport the entry seed is the ONLY thing that ever writes the pane's pre-entry output into the grid, so it must ask for everything the grid can hold (issue #29)", got, ScrollbackMaxLines)
	}
	if got := EntrySeedHistoryLines(TransportCapture); got != 0 {
		t.Errorf("EntrySeedHistoryLines(TransportCapture) = %d, want 0: captureLoop replaces the whole grid from a visible-only capture within one capturePollInterval, so history pulled at entry is discarded 200ms later -- for ~15ms of capture plus ~94ms of grid writing on bubbletea's blocking Update goroutine", got)
	}
	if got, want := tmux.SeedCaptureOptionsWithHistory(EntrySeedHistoryLines(TransportCapture)), tmux.SeedCaptureOptions(); got != want {
		t.Errorf("the capture transport's entry seed range = %+v, want exactly tmux.SeedCaptureOptions() %+v (the same range, and the same cost, the entry seed used before issue #29)", got, want)
	}
}

// TestCaptureTransportFirstTickDiscardsAnyHistoryTheEntrySeedPulled is the
// non-vacuity control for the zero above: it demonstrates, against a real
// tmux pane with real scrollback, that history pulled by a
// TransportCapture Session's entry seed does not survive even one
// capturePollInterval. Without this the zero would be an unexplained
// magic number, and a future reader could "fix" it back to
// ScrollbackMaxLines and see nothing at all go wrong -- every existing
// test in this package and in internal/tui would still pass, because the
// waste is invisible in behaviour and shows up only as latency on Enter
// and garbage on the heap.
//
// The seed closure here deliberately asks for ScrollbackMaxLines -- the
// WASTEFUL value, not EntrySeedHistoryLines' answer -- because the point
// is to show what that value buys under this transport: a scrollback that
// is populated at the moment the grid is built and empty a fifth of a
// second later.
func TestCaptureTransportFirstTickDiscardsAnyHistoryTheEntrySeedPulled(t *testing.T) {
	const width, height = 40, 10
	original := capturePollInterval
	capturePollInterval = 40 * time.Millisecond
	defer func() { capturePollInterval = original }()

	socket := interactiveSocket("capture-discards-history")
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	fillPaneHistory(t, socket, "s0", height*6, true)
	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// The premise, measured before the Session exists so the loop cannot
	// have raced it: a history-inclusive entry seed really does populate
	// the grid's scrollback for this pane.
	seed, err := CaptureSeedWithHistory(ctx, client, paneID, ScrollbackMaxLines)
	if err != nil {
		t.Fatalf("CaptureSeedWithHistory: %v", err)
	}
	entryGrid := newGrid(width, height)
	if _, err := entryGrid.Write(seed); err != nil {
		t.Fatalf("write history-inclusive entry seed: %v", err)
	}
	seeded := entryGrid.ScrollbackLen()
	if seeded <= 0 {
		t.Fatalf("the history-inclusive entry seed left ScrollbackLen() = %d; the fixture has no history, so there is nothing for the capture loop to discard and this test proves nothing", seeded)
	}

	session, err := StartWithTransport(ctx, client, paneID, width, height, func(ctx context.Context) ([]byte, error) {
		return CaptureSeedWithHistory(ctx, client, paneID, ScrollbackMaxLines)
	}, TransportCapture)
	if err != nil {
		t.Fatalf("StartWithTransport(..., TransportCapture): %v", err)
	}
	defer session.Close()

	// Several ticks, so this is a settled state rather than a snapshot
	// taken mid-replacement.
	time.Sleep(6 * capturePollInterval)
	if got := session.Grid().ScrollbackLen(); got != 0 {
		t.Errorf("after %d capture ticks the grid's ScrollbackLen() = %d, want 0: captureLoop is expected to have replaced the entry seed's %d-line scrollback wholesale from a visible-only capture, which is why EntrySeedHistoryLines(TransportCapture) is 0. If this now holds history, re-derive that answer instead of changing this test", 6, got, seeded)
	}
}
