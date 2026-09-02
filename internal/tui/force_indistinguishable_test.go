package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// ownershipClaimShapeRe matches OwnershipOption's documented `<tag>:<pid>`
// form (internal/tmux/ownership.go's formatOwnershipClaim): a 16-hex-char
// tag, a literal colon, then the claiming process's decimal pid. Neither
// ClaimWindowOwnership nor ForceClaimWindowOwnership is reachable from
// package tui to parse this directly (parseOwnershipClaim is unexported),
// so this test reads the option straight off the wire, the same way
// internal/tmux's own ownership_test.go's readTmuxOptionForOwnershipTest
// does from inside that package.
var ownershipClaimShapeRe = regexp.MustCompile(`^([0-9a-f]{16}):([0-9]+)$`)

// readWindowOwnershipOptionForTest reads tmux.OwnershipOption's raw value
// off target on socket, the same "invalid option" == unset distinction
// internal/tmux/geometry.go's readWindowOwnership already makes.
func readWindowOwnershipOptionForTest(t *testing.T, socket, target string) (string, bool) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-options", "-wv", "-t", target, tmux.OwnershipOption).CombinedOutput()
	trimmed := strings.TrimRight(string(out), "\n")
	if err != nil {
		if strings.Contains(trimmed, "invalid option") {
			return "", false
		}
		t.Fatalf("show-options -wv -t %s %s: %v: %s", target, tmux.OwnershipOption, err, trimmed)
	}
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// paneSizeForTest reads paneID's own #{pane_width}x#{pane_height}, the
// same pair Client.FitWindowToPane converges against internally.
func paneSizeForTest(t *testing.T, socket, paneID string) (width, height int) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", paneID, "#{pane_width}x#{pane_height}").CombinedOutput()
	if err != nil {
		t.Fatalf("display-message pane size for %s: %v: %s", paneID, err, out)
	}
	fields := strings.SplitN(strings.TrimSpace(string(out)), "x", 2)
	if len(fields) != 2 {
		t.Fatalf("display-message pane size for %s: unexpected output %q", paneID, out)
	}
	w, errW := strconv.Atoi(fields[0])
	h, errH := strconv.Atoi(fields[1])
	if errW != nil || errH != nil {
		t.Fatalf("parse pane size %q: %v / %v", out, errW, errH)
	}
	return w, h
}

// TestForceOnUncontendedWindowMatchesReturnKey proves task 108's whole
// point: on a window with nothing to steal (no attached client, no live
// claim), `F` (enterInteractiveBody(true)) is indistinguishable from `↵`
// (enterInteractiveBody(false)) in every observable this ladder produces
// -- the fitted window size, the claim option's own `<tag>:<pid>` shape,
// the resize-window count Client.FitWindowToPane reports (which
// enterInteractiveBody itself discards, so this measures it directly
// against two panes with identical starting geometry and the identical
// wanted size both entries compute from the same m.width/m.height), and
// exactly one prepareAttach call each (SPEC §7).
func TestForceOnUncontendedWindowMatchesReturnKey(t *testing.T) {
	probe := New(nil, config.Settings{Color: true}, "")
	probe.width, probe.height = 100, 30
	wantWidth, wantHeight := probe.previewContentSize()
	if wantHeight < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", wantHeight, interactiveMinInnerRows)
	}

	enterSocket := selectionTestSocket("indistkey")
	forceSocket := selectionTestSocket("indistforce")
	newQuietSelectionPane(t, enterSocket, "deck_indistkey", 80, 24)
	newQuietSelectionPane(t, forceSocket, "deck_indistforce", 80, 24)
	enterClient := tmux.Client{Socket: enterSocket}
	forceClient := tmux.Client{Socket: forceSocket}

	enterTarget, err := tmux.SessionName("indistkey")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	forceTarget, err := tmux.SessionName("indistforce")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	ctx := context.Background()
	enterPane, ok, err := enterClient.PreviewPane(ctx, "indistkey")
	if err != nil || !ok {
		t.Fatalf("PreviewPane(indistkey): ok=%v err=%v", ok, err)
	}
	forcePane, ok, err := forceClient.PreviewPane(ctx, "indistforce")
	if err != nil || !ok {
		t.Fatalf("PreviewPane(indistforce): ok=%v err=%v", ok, err)
	}

	// Measure the resize-window count Client.FitWindowToPane reports for
	// each pane BEFORE either entry: both panes started at the identical
	// 80x24 geometry and both are fit to the identical wantWidth/
	// wantHeight, so the two counts are directly comparable, and this is
	// the one place either count is ever observed at all -- the real
	// entries below call this exact same method internally but discard
	// its return value.
	enterResizes, err := enterClient.FitWindowToPane(ctx, enterTarget, enterPane.ID, wantWidth, wantHeight)
	if err != nil {
		t.Fatalf("measure fit resize count (↵ pane): %v", err)
	}
	forceResizes, err := forceClient.FitWindowToPane(ctx, forceTarget, forcePane.ID, wantWidth, wantHeight)
	if err != nil {
		t.Fatalf("measure fit resize count (F pane): %v", err)
	}
	if enterResizes == 0 {
		t.Fatalf("test assumption violated: fitting an 80x24 pane to %dx%d took 0 resize-window calls", wantWidth, wantHeight)
	}
	if enterResizes != forceResizes {
		t.Fatalf("resize-window count differs for identical starting/target geometry: ↵ pane measured %d, F pane measured %d", enterResizes, forceResizes)
	}

	// ↵ entry, on an uncontended window (no attached client, no claim).
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = enterClient
	m.sessions = []store.Session{{ID: "sess-indist-enter", Name: "indistkey", Slug: "indistkey", Status: "waiting"}}
	m.selected = 0
	var enterRecorded []string
	m.prepareAttach = func(_ context.Context, id string) error {
		enterRecorded = append(enterRecorded, id)
		return nil
	}
	next, _ := m.enterInteractive()
	enterGot := next.(Model)
	if enterGot.attachError != "" {
		t.Fatalf("↵ entry refused on an uncontended window: %q", enterGot.attachError)
	}
	if !enterGot.interactive {
		t.Fatalf("↵ entry did not enter interactive mode")
	}
	if len(enterRecorded) != 1 || enterRecorded[0] != "sess-indist-enter" {
		t.Fatalf("↵ entry's prepareAttach calls = %v, want exactly one with the durable row's ID %q", enterRecorded, "sess-indist-enter")
	}
	enterOption, ok := readWindowOwnershipOptionForTest(t, enterSocket, enterTarget)
	if !ok {
		t.Fatalf("↵ entry left no ownership option set on %q", enterTarget)
	}
	enterMatch := ownershipClaimShapeRe.FindStringSubmatch(enterOption)
	if enterMatch == nil {
		t.Fatalf("↵ entry's claim %q is not in the <tag>:<pid> shape", enterOption)
	}
	enterFinalWidth, enterFinalHeight := paneSizeForTest(t, enterSocket, enterPane.ID)
	enterGot.exitInteractive()

	// F entry, on its OWN uncontended window -- same wanted size, same
	// starting geometry, same process.
	m2 := New(nil, config.Settings{Color: true}, "")
	m2.width, m2.height = 100, 30
	m2.tmuxClient = forceClient
	m2.sessions = []store.Session{{ID: "sess-indist-force", Name: "indistforce", Slug: "indistforce", Status: "waiting"}}
	m2.selected = 0
	var forceRecorded []string
	m2.prepareAttach = func(_ context.Context, id string) error {
		forceRecorded = append(forceRecorded, id)
		return nil
	}
	next2, _ := m2.enterInteractiveBody(true)
	forceGot := next2.(Model)
	if forceGot.attachError != "" {
		t.Fatalf("F entry refused on an uncontended window: %q", forceGot.attachError)
	}
	if !forceGot.interactive {
		t.Fatalf("F entry did not enter interactive mode")
	}
	if len(forceRecorded) != 1 || forceRecorded[0] != "sess-indist-force" {
		t.Fatalf("F entry's prepareAttach calls = %v, want exactly one with the durable row's ID %q", forceRecorded, "sess-indist-force")
	}
	forceOption, ok := readWindowOwnershipOptionForTest(t, forceSocket, forceTarget)
	if !ok {
		t.Fatalf("F entry left no ownership option set on %q", forceTarget)
	}
	forceMatch := ownershipClaimShapeRe.FindStringSubmatch(forceOption)
	if forceMatch == nil {
		t.Fatalf("F entry's claim %q is not in the <tag>:<pid> shape", forceOption)
	}
	forceFinalWidth, forceFinalHeight := paneSizeForTest(t, forceSocket, forcePane.ID)
	forceGot.exitInteractive()

	if enterFinalWidth != forceFinalWidth || enterFinalHeight != forceFinalHeight {
		t.Fatalf("fitted window size differs on an uncontended window: ↵ = %dx%d, F = %dx%d", enterFinalWidth, enterFinalHeight, forceFinalWidth, forceFinalHeight)
	}
	if enterFinalWidth != wantWidth || enterFinalHeight != wantHeight {
		t.Fatalf("fitted window size %dx%d does not match the wanted preview content size %dx%d", enterFinalWidth, enterFinalHeight, wantWidth, wantHeight)
	}

	wantPID := strconv.Itoa(os.Getpid())
	if enterMatch[2] != wantPID {
		t.Fatalf("↵ claim's pid = %s, want this process's own pid %s", enterMatch[2], wantPID)
	}
	if forceMatch[2] != wantPID {
		t.Fatalf("F claim's pid = %s, want this process's own pid %s", forceMatch[2], wantPID)
	}
}

// TestRefusedForceRecordsNoPrepareAttach proves the other half of task
// 108: when Client.ForceClaimWindowOwnership's own single-shot confirm-read
// loses to a genuinely concurrent writer (the one way a force claim is
// ever refused -- ForceClaimWindowOwnership never consults pidAlive, so
// nothing else can make it stand down), enterInteractiveBody(true) returns
// the same "a live process holds ownership" refusal the ordinary claim
// path uses and never reaches m.prepareAttach at all. A busy interloper
// goroutine hammers the same window's OwnershipOption with `set-option`
// for the whole span of one enterInteractiveBody(true) call -- across
// several tmux round trips (PreviewPane, CaptureWindowGeometry, the claim
// itself) -- so at least one of its writes has a real chance of landing
// between the claim's own write and its confirm-read; this repeats fresh
// rounds, exactly like internal/tmux/force_ownership_test.go's own
// TestForceClaimWindowOwnershipConfirmReadLosesToACompetingWriter, and
// fails only if no round out of a generous budget ever produces that
// interleaving.
func TestRefusedForceRecordsNoPrepareAttach(t *testing.T) {
	socket := selectionTestSocket("indistrefuse")
	newQuietSelectionPane(t, socket, "deck_indistrefuse", 80, 24)
	client := tmux.Client{Socket: socket}
	target, err := tmux.SessionName("indistrefuse")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	const maxRounds = 100
	for round := 0; round < maxRounds; round++ {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = 100, 30
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-indist-refuse", Name: "indistrefuse", Slug: "indistrefuse", Status: "waiting"}}
		m.selected = 0
		var recorded []string
		m.prepareAttach = func(_ context.Context, id string) error {
			recorded = append(recorded, id)
			return nil
		}

		stop := make(chan struct{})
		done := make(chan struct{})
		go func(round int) {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = exec.Command("tmux", "-L", socket, "set-option", "-w", "-t", target, tmux.OwnershipOption, fmt.Sprintf("interloper%d:99999999", round)).Run()
			}
		}(round)

		next, _ := m.enterInteractiveBody(true)
		close(stop)
		<-done
		got := next.(Model)

		if got.interactive {
			// This round's confirm-read was not raced away (the
			// interloper's writes all landed outside the narrow
			// write/confirm-read window, or all before the claim's own
			// write) -- try another round.
			got.exitInteractive()
			continue
		}
		if !strings.Contains(got.attachError, "a live process holds ownership of this window") {
			// A different refusal (a transport hiccup unrelated to the
			// force-claim race) -- not the property this test needs;
			// try another round.
			continue
		}
		if len(recorded) != 0 {
			t.Fatalf("round %d: refused force recorded %d prepareAttach call(s), want 0: %v", round, len(recorded), recorded)
		}
		return
	}
	t.Fatalf("no round out of %d produced a genuine force-claim confirm-read loss to a concurrent writer; widen maxRounds", maxRounds)
}
