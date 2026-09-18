// paneseed_atomic_test.go proves PRD phase3b II-20: state and body are
// paired atomically by chaining them into one tmux invocation and
// retrying while the before/after #{history_size}/#{pane_width}/
// #{pane_height} probes disagree.
package tmux

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// resolveSolePaneID resolves the single pane of a freshly created bare
// session to its real "%N" id -- CapturePaneSeedAtomic enforces the same
// pane-id-shaped target CapturePane already requires, unlike
// PaneSeedState (which many other tests in this package happily target by
// session name). Client.List only ever returns deck_-prefixed sessions
// (its own filter), which newBareGeometrySession deliberately does not
// create, so this asks tmux directly instead.
func resolveSolePaneID(t *testing.T, socket, session string) string {
	t.Helper()
	return runTmux(t, socket, "list-panes", "-t", session, "-F", "#{pane_id}")
}

// TestCapturePaneSeedAtomicMatchesSeparateStateAndBodyReads proves the
// happy path against a real, quiescent pane: chaining the two reads into
// one invocation must not change what either read reports compared to
// calling PaneSeedState and CapturePane separately (the pre-II-20 shape
// CaptureSeed used).
func TestCapturePaneSeedAtomicMatchesSeparateStateAndBodyReads(t *testing.T) {
	socket := geometrySocket("seed-atomic-match")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	runInPaneBlocking(t, socket, "s0", `\033[3;7r\033[?6h\033[31mRED\033[0m`)
	paneID := resolveSolePaneID(t, socket, "s0")

	wantState, err := client.PaneSeedState(ctx, paneID)
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}
	options := SeedCaptureOptions()
	wantBody, err := client.CapturePane(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	// PLAIN byte equality, with nothing trimmed off either side (issue
	// #29). It used to be a comparison against
	// bytes.TrimSuffix(wantBody, "\n"), because the atomic path's body was
	// reconstructed WITHOUT the one trailing newline capture-pane emits
	// after its last row -- the chained after-probe line consumed it as
	// its own separator. capturePaneSeedAtomicOnce re-appends it now, so
	// the two producers are byte-identical and this test is strictly
	// stronger than it was; that agreement is what lets
	// internal/interactive.BuildSeed have ONE correct trim rule
	// (bytes.TrimSuffix of exactly the terminator) instead of a rule that
	// had to cope with both shapes and therefore ate the body's whole
	// trailing blank-row run.
	//
	// Preserving the insight the old trim's comment carried, because it is
	// still exactly why BuildSeed must not use bytes.TrimRight and why
	// TestCapturePaneSeedAtomicPreservesATrailingRunOfBlankRows exists: a
	// TrimRight-shaped trim eats not just the terminator but every
	// interior-but-trailing BLANK ROW that -N deliberately preserves as a
	// bare empty line, and it does so silently -- the result still looks
	// like a plausible capture, just K rows shorter than the pane is tall.

	gotState, gotBody, err := client.CapturePaneSeedAtomic(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic: %v", err)
	}

	if gotState != wantState {
		t.Errorf("CapturePaneSeedAtomic state = %+v, want %+v (matching separate PaneSeedState)", gotState, wantState)
	}
	if string(gotBody) != string(wantBody) {
		t.Errorf("CapturePaneSeedAtomic body = %q, want %q (matching separate CapturePane)", gotBody, wantBody)
	}
}

// TestCapturePaneSeedAtomicRejectsInvalidPaneID mirrors CapturePane's own
// validation (tmux_test.go's capture-pane range tests): the target must
// be a real pane id, since this issues the identical capture-pane call
// CapturePane does, chained alongside the state reads.
func TestCapturePaneSeedAtomicRejectsInvalidPaneID(t *testing.T) {
	client := Client{Socket: "deck-atomic-invalid-target"}
	if _, _, err := client.CapturePaneSeedAtomic(context.Background(), "s0", SeedCaptureOptions()); err == nil {
		t.Fatalf("CapturePaneSeedAtomic with a session-name target: want error, got nil")
	}
}

// TestSeparateInvocationsCanObserveHistorySizeChangeBetweenThem is the
// mandatory negative control this task's success criteria names: proof
// that two SEPARATE tmux invocations -- the shape CaptureSeed used before
// this task, and the general hazard II-20 exists to close -- can each see
// a different value for the same discriminator, because tmux processes a
// pane's own output in the gap between them. A pane floods its own
// scrollback via a backgrounded `yes`, and two back-to-back
// display-message calls for #{history_size} a short, fixed pause apart
// are asserted to disagree.
func TestSeparateInvocationsCanObserveHistorySizeChangeBetweenThem(t *testing.T) {
	socket := geometrySocket("seed-atomic-interleave")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 5)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// An UNTHROTTLED `yes` was tried first and rejected: it saturates the
	// host CPU badly enough (measured) that the two client-side tmux
	// invocations below can themselves be scheduled out of the order they
	// were issued in, occasionally making history_size go DOWN instead of
	// up between them -- a scheduling artefact of the test harness, not
	// evidence about tmux. A throttled loop still grows history reliably
	// inside the 200ms window without starving the host. Backgrounding it
	// and blocking on `cat` (the same pattern runInPaneBlocking uses)
	// keeps the shell from ever redrawing a prompt that would perturb
	// anything else.
	runTmux(t, socket, "send-keys", "-t", "s0", "-l", "--", "(while :; do echo flood-history-so-two-separate-probes-disagree; sleep 0.01; done) & cat > /dev/null")
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	time.Sleep(300 * time.Millisecond)

	first, err := client.run(ctx, "display-message", "-p", "-t", "s0", "#{history_size}")
	if err != nil {
		t.Fatalf("first display-message: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	second, err := client.run(ctx, "display-message", "-p", "-t", "s0", "#{history_size}")
	if err != nil {
		t.Fatalf("second display-message: %v", err)
	}

	firstSize, err := strconv.Atoi(strings.TrimSpace(string(first)))
	if err != nil {
		t.Fatalf("parse first history_size %q: %v", first, err)
	}
	secondSize, err := strconv.Atoi(strings.TrimSpace(string(second)))
	if err != nil {
		t.Fatalf("parse second history_size %q: %v", second, err)
	}
	if secondSize <= firstSize {
		t.Fatalf("two separate invocations 200ms apart against a flooding pane saw history_size %d then %d; want the second strictly greater, proving the pane's own output interleaves between separate invocations (test setup or timing assumption is wrong if this fails)", firstSize, secondSize)
	}
}

// fakeAtomicPaneSeedScript writes a fake tmux binary that ignores every
// argument it is given and instead answers ANY invocation with the
// chained three-line shape capturePaneSeedAtomicOnce expects to parse:
// a state+before-probe line, two body lines, and an after-probe line.
// The first N-1 calls report a before/after mismatch (a disagreement);
// the Nth call reports agreement, so CapturePaneSeedAtomic's retry loop
// is exercised for exactly N-1 forced extra attempts before it succeeds.
// counterPath is a plain text file (not a tmux state), used only so the
// script itself can tell which call this is.
func fakeAtomicPaneSeedScript(t *testing.T, disagreeingCalls int) (binary, counterPath string) {
	t.Helper()
	dir := t.TempDir()
	counterPath = filepath.Join(dir, "calls")
	if err := os.WriteFile(counterPath, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary = filepath.Join(dir, "tmux")
	// Sixteen fields matching paneSeedStateFieldCount, all zero/false
	// except a couple of Atoi-parseable ints -- their values don't matter
	// to this test, only that they parse and that before/after either
	// match or don't per the call count.
	const stateSixteen = "0|0|0|1|0|0|0|0|0|0|0|0|1|0|0|9"
	// Four discriminator fields, not three: #{alternate_on} became the
	// fourth for issue #29 (paneSeedDiscriminatorFormat says why). Its
	// value is 0 on both probes here so this fixture keeps testing exactly
	// what it used to -- a #{history_size} disagreement -- and so
	// CapturePaneSeedAtomic's alt-screen RE-CAPTURE stays out of the
	// picture (the state line's first field, AlternateOn, is 0 too, so the
	// invocation counts below are the retry loop's alone).
	script := `#!/bin/sh
n=$(cat "` + counterPath + `")
n=$((n + 1))
echo "$n" > "` + counterPath + `"
if [ "$n" -le ` + strconv.Itoa(disagreeingCalls) + ` ]; then
  before="5|40|10|0"
  after="6|40|10|0"
else
  before="7|40|10|0"
  after="7|40|10|0"
fi
printf '%s|%s\n' "` + stateSixteen + `" "$before"
printf 'fake-body-line-1\nfake-body-line-2\n'
printf '%s\n' "$after"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, counterPath
}

// TestCapturePaneSeedAtomicRetriesWhileProbesDisagree is the mandatory
// deterministic proof that the retry actually fires: a fake tmux binary
// disagrees on its first call and agrees on its second, and
// CapturePaneSeedAtomic is asserted to (a) succeed anyway, (b) have
// invoked the fake binary exactly twice, and (c) return the SECOND call's
// (agreeing) state rather than silently keeping the first, mismatched
// one.
func TestCapturePaneSeedAtomicRetriesWhileProbesDisagree(t *testing.T) {
	binary, counterPath := fakeAtomicPaneSeedScript(t, 1)
	client := Client{Binary: binary, Socket: "deck-atomic-retry", Timeout: 5 * time.Second}

	state, body, err := client.CapturePaneSeedAtomic(context.Background(), "%0", SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic: %v", err)
	}

	calls, readErr := os.ReadFile(counterPath)
	if readErr != nil {
		t.Fatalf("read call counter: %v", readErr)
	}
	if got := strings.TrimSpace(string(calls)); got != "2" {
		t.Fatalf("fake tmux invocation count = %s, want exactly 2 (one disagreement forcing exactly one retry)", got)
	}
	if state.ScrollRegionLower != 9 {
		t.Fatalf("state.ScrollRegionLower = %d, want 9 from the fabricated fixture (proves the SECOND, agreeing call's state was kept)", state.ScrollRegionLower)
	}
	// The trailing "\n" is part of the expectation now (issue #29): the
	// body a caller gets back is byte-identical to what capture-pane
	// itself printed, terminator included, which is exactly what the
	// fixture's own printf emitted.
	if string(body) != "fake-body-line-1\nfake-body-line-2\n" {
		t.Fatalf("body = %q, want the fabricated fixture's two lines with capture-pane's own trailing newline", body)
	}
}

// TestCapturePaneSeedAtomicGivesUpAfterMaxAttempts proves the retry loop
// is bounded: a fake tmux binary that disagrees on every single call
// (well past maxPaneSeedAtomicAttempts) makes CapturePaneSeedAtomic
// return an error naming the exhausted attempt count, rather than
// retrying forever.
func TestCapturePaneSeedAtomicGivesUpAfterMaxAttempts(t *testing.T) {
	binary, counterPath := fakeAtomicPaneSeedScript(t, maxPaneSeedAtomicAttempts+10)
	client := Client{Binary: binary, Socket: "deck-atomic-exhausted", Timeout: 5 * time.Second}

	_, _, err := client.CapturePaneSeedAtomic(context.Background(), "%0", SeedCaptureOptions())
	if err == nil {
		t.Fatalf("CapturePaneSeedAtomic against a permanently disagreeing fake tmux: want error, got nil")
	}
	if !strings.Contains(err.Error(), "probes never agreed") {
		t.Fatalf("error = %v, want it to name the exhausted-retries condition", err)
	}

	calls, readErr := os.ReadFile(counterPath)
	if readErr != nil {
		t.Fatalf("read call counter: %v", readErr)
	}
	if got := strings.TrimSpace(string(calls)); got != strconv.Itoa(maxPaneSeedAtomicAttempts) {
		t.Fatalf("fake tmux invocation count = %s, want exactly maxPaneSeedAtomicAttempts (%d)", got, maxPaneSeedAtomicAttempts)
	}
}

// TestParsePaneSeedDiscriminatorsRejectsWrongFieldCount guards the wire
// format the same way TestParsePaneSeedStateRejectsWrongFieldCount does
// for the full state. The three-field case is the one that matters since
// issue #29: three fields is exactly what this format USED to be, so a
// future edit that drops #{alternate_on} from
// paneSeedDiscriminatorFormat without dropping it from
// parsePaneSeedDiscriminators (or vice versa) fails loudly here instead
// of silently reading a pane's alternate-screen flag out of nothing.
func TestParsePaneSeedDiscriminatorsRejectsWrongFieldCount(t *testing.T) {
	if _, err := parsePaneSeedDiscriminators("0|0"); err == nil {
		t.Fatalf("parsePaneSeedDiscriminators with too few fields: want error, got nil")
	}
	if _, err := parsePaneSeedDiscriminators("5|40|10"); err == nil {
		t.Fatalf("parsePaneSeedDiscriminators with the pre-issue-#29 THREE fields: want error, got nil")
	}
	got, err := parsePaneSeedDiscriminators("5|40|10|1")
	if err != nil {
		t.Fatalf("parsePaneSeedDiscriminators with four fields: %v", err)
	}
	if want := (paneSeedDiscriminators{HistorySize: 5, PaneWidth: 40, PaneHeight: 10, AlternateOn: 1}); got != want {
		t.Fatalf("parsePaneSeedDiscriminators = %+v, want %+v (field ORDER must match paneSeedDiscriminatorFormat)", got, want)
	}
}

// TestCapturePaneSeedAtomicPreservesATrailingRunOfBlankRows is issue #29's
// producer-agreement test in its sharpest form: a pane whose visible
// screen ends in a long run of BLANK rows -- the ordinary state of any
// pane whose program has printed less than a screenful -- must come back
// from CapturePaneSeedAtomic byte-identically to what Client.CapturePane
// returns for the same options, trailing newline and every blank row
// included.
//
// It is the trailing blank rows that make this test able to fail. The two
// producers used to disagree by exactly one newline (the atomic path's
// reconstruction consumed capture-pane's terminator as the separator
// before the chained after-probe line), which forced
// internal/interactive.BuildSeed to trim with bytes.TrimRight to cope
// with both shapes -- and TrimRight does not stop at the terminator, it
// eats the entire trailing run of bare empty lines that `-N` exists to
// preserve. Harmless while the body was the visible screen only; fatal
// with history prepended, where the body is bottom-anchored and dropping
// K blank rows shifts the live screen K rows up into scrollback.
func TestCapturePaneSeedAtomicPreservesATrailingRunOfBlankRows(t *testing.T) {
	socket := geometrySocket("seed-atomic-blank-tail")
	const height = 20
	cleanup := newBareGeometrySession(t, socket, "s0", 40, height)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Two rows of content at the top of a 20-row pane, then a blocking
	// `cat` so the pane is quiescent (the two captures below must see the
	// identical pane) and the shell never redraws a prompt further down.
	// Row 0 gets the echoed command itself, so the pane ends up with a
	// dozen-plus genuinely blank rows underneath.
	runInPaneBlocking(t, socket, "s0", `TOP-ROW-CONTENT\n`)
	paneID := resolveSolePaneID(t, socket, "s0")

	options := SeedCaptureOptions()
	want, err := client.CapturePane(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	// Confirm the premise: the body really does end in a RUN of blank
	// rows, so a whole-run trim would be observable here.
	trailingBlanks := 0
	for _, row := range bytes.Split(bytes.TrimSuffix(want, []byte("\n")), []byte("\n")) {
		if len(bytes.TrimRight(row, " ")) == 0 {
			trailingBlanks++
		} else {
			trailingBlanks = 0
		}
	}
	if trailingBlanks < 2 {
		t.Fatalf("fixture pane ends in %d blank row(s), want a RUN of them; this test cannot distinguish trimming one newline from trimming the whole tail:\n%q", trailingBlanks, want)
	}

	_, got, err := client.CapturePaneSeedAtomic(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePaneSeedAtomic: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("CapturePaneSeedAtomic body is not byte-identical to CapturePane's for the same options\n got %d rows, %d bytes: %q\nwant %d rows, %d bytes: %q",
			bytes.Count(got, []byte("\n")), len(got), got,
			bytes.Count(want, []byte("\n")), len(want), want)
	}
	if rows := bytes.Count(got, []byte("\n")); rows != height {
		t.Errorf("visible-only body = %d rows, want exactly pane_height (%d): -N must emit the pane's trailing blank rows too", rows, height)
	}
}

// fakeAlternateOnFlipScript writes a fake tmux binary that answers every
// invocation with the chained shape capturePaneSeedAtomicOnce parses, and
// whose before/after probes differ in NOTHING BUT #{alternate_on} on the
// first `flippingCalls` calls: the before probe always reports
// beforeAlternateOn, the after probe reports flippedAlternateOn while
// calls remain to flip and beforeAlternateOn (i.e. agreement) from then
// on. It is the alt-screen twin of fakeAtomicPaneSeedScript, which
// disagrees on #{history_size} instead.
func fakeAlternateOnFlipScript(t *testing.T, flippingCalls int, beforeAlternateOn, flippedAlternateOn string) (binary, counterPath string) {
	t.Helper()
	dir := t.TempDir()
	counterPath = filepath.Join(dir, "calls")
	if err := os.WriteFile(counterPath, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary = filepath.Join(dir, "tmux")
	// The state line's own #{alternate_on} (its FIRST field) stays 0
	// throughout: this fixture is about the DISCRIMINATOR, and a state
	// that also claimed the alternate screen would drag
	// CapturePaneSeedAtomic's visible-only re-capture into the same test
	// and add an invocation the counts below would then have to explain.
	const stateSixteen = "0|0|0|1|0|0|0|0|0|0|0|0|1|0|0|9"
	script := `#!/bin/sh
n=$(cat "` + counterPath + `")
n=$((n + 1))
echo "$n" > "` + counterPath + `"
before="7|40|10|` + beforeAlternateOn + `"
if [ "$n" -le ` + strconv.Itoa(flippingCalls) + ` ]; then
  after="7|40|10|` + flippedAlternateOn + `"
else
  after="7|40|10|` + beforeAlternateOn + `"
fi
printf '%s|%s\n' "` + stateSixteen + `" "$before"
printf 'fake-body-line-1\nfake-body-line-2\n'
printf '%s\n' "$after"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, counterPath
}

// TestCapturePaneSeedAtomicRetriesWhenOnlyAlternateOnDisagrees is the
// deterministic proof that #{alternate_on} really is a discriminator
// (issue #29), mirroring TestCapturePaneSeedAtomicRetriesWhileProbesDisagree
// for the field it added. history_size, pane_width and pane_height all
// AGREE across the fake's probes, so the retry can only be caused by the
// alternate-screen flag having flipped.
//
// The direction that matters is the harmful one: a pane that switches to
// its alternate screen between the state read and the capture reports
// AlternateOn=false -- so no visible-only re-capture is triggered -- over
// a body that IS an alternate screen with the pane's stale primary-screen
// history above it. Retrying is the only correct answer, since the range
// itself was already committed to before the invocation ran.
func TestCapturePaneSeedAtomicRetriesWhenOnlyAlternateOnDisagrees(t *testing.T) {
	binary, counterPath := fakeAlternateOnFlipScript(t, 1, "0", "1")
	client := Client{Binary: binary, Socket: "deck-atomic-alt-flip", Timeout: 5 * time.Second}

	if _, _, err := client.CapturePaneSeedAtomic(context.Background(), "%0", SeedCaptureOptionsWithHistory(2000)); err != nil {
		t.Fatalf("CapturePaneSeedAtomic: %v", err)
	}
	calls, readErr := os.ReadFile(counterPath)
	if readErr != nil {
		t.Fatalf("read call counter: %v", readErr)
	}
	if got := strings.TrimSpace(string(calls)); got != "2" {
		t.Fatalf("fake tmux invocation count = %s, want exactly 2 (one alternate_on-only disagreement forcing exactly one retry); a count of 1 means #{alternate_on} is not being compared", got)
	}
}

// TestCapturePaneSeedAtomicDoesNotRetryWhenAlternateOnAgrees is the other
// half of the control above: with all four discriminators agreeing on the
// first call -- alternate_on included, and set to 1 on BOTH probes so the
// test cannot pass merely because the field is always zero -- there is
// exactly ONE invocation. Without this, a "discriminator" that always
// disagreed would look just as green as one that compares correctly.
func TestCapturePaneSeedAtomicDoesNotRetryWhenAlternateOnAgrees(t *testing.T) {
	binary, counterPath := fakeAlternateOnFlipScript(t, 0, "1", "0")
	client := Client{Binary: binary, Socket: "deck-atomic-alt-agree", Timeout: 5 * time.Second}

	if _, _, err := client.CapturePaneSeedAtomic(context.Background(), "%0", SeedCaptureOptionsWithHistory(2000)); err != nil {
		t.Fatalf("CapturePaneSeedAtomic: %v", err)
	}
	calls, readErr := os.ReadFile(counterPath)
	if readErr != nil {
		t.Fatalf("read call counter: %v", readErr)
	}
	if got := strings.TrimSpace(string(calls)); got != "1" {
		t.Fatalf("fake tmux invocation count = %s, want exactly 1 (every discriminator agrees on the first call)", got)
	}
}
