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
	// CapturePane's raw output keeps its own trailing "\n" after the last
	// row; the atomic path's body is reconstructed without exactly that
	// ONE trailing newline, because the chained after-probe line consumes
	// it as its own separator instead (see capturePaneSeedAtomicOnce's doc
	// comment). Trim exactly one trailing "\n" here too -- NOT
	// strings.TrimRight, which would also eat any interior-but-trailing
	// BLANK ROWS -N preserves as bare empty lines, silently making this
	// comparison pass even if -N's effect were lost somewhere.
	wantBody = bytes.TrimSuffix(wantBody, []byte("\n"))

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
	script := `#!/bin/sh
n=$(cat "` + counterPath + `")
n=$((n + 1))
echo "$n" > "` + counterPath + `"
if [ "$n" -le ` + strconv.Itoa(disagreeingCalls) + ` ]; then
  before="5|40|10"
  after="6|40|10"
else
  before="7|40|10"
  after="7|40|10"
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
	if string(body) != "fake-body-line-1\nfake-body-line-2" {
		t.Fatalf("body = %q, want the fabricated fixture's two lines", body)
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
// for the full state.
func TestParsePaneSeedDiscriminatorsRejectsWrongFieldCount(t *testing.T) {
	if _, err := parsePaneSeedDiscriminators("0|0"); err == nil {
		t.Fatalf("parsePaneSeedDiscriminators with too few fields: want error, got nil")
	}
}
