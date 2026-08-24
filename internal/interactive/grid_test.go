// grid_test.go proves PRD phase3b II-16 directly: arming pipe-pane after
// the seed capture loses interstitial bytes with no way to notice, and
// arming it first does not.
package interactive

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

func interactiveSocket(name string) string {
	return fmt.Sprintf("deck-interactive-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

func newBareInteractiveSession(t *testing.T, socket, session string, width, height int) (cleanup func()) {
	t.Helper()
	args := []string{"-L", socket, "new-session", "-d", "-s", session, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start bare tmux session: %v: %s", err, out)
	}
	return func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	}
}

// sendLiteralLine types literal into the pane followed by Enter, so the
// shell echoes it back and it becomes part of the pane's rendered screen
// exactly like any other keystroke-driven output would.
func sendLiteralLine(t *testing.T, socket, target, literal string) {
	t.Helper()
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "-l", "--", literal).CombinedOutput(); err != nil {
		t.Fatalf("send-keys -l -- %q: %v: %s", literal, err, out)
	}
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("send-keys Enter: %v: %s", err, out)
	}
}

func rawCapturePane(t *testing.T, socket, target string) []byte {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		t.Fatalf("capture-pane -p -t %s: %v", target, err)
	}
	return out
}

// ansiEscapeRe strips the CSI (SGR) and OSC (hyperlink) escape sequences
// that Render() interleaves between styled cell runs, matching the same
// two shapes composed_only_test.go's dangerousANSIClasses already greps
// for (CSI ... final-byte, and OSC ... BEL/ST).
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9:;]*[A-Za-z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// gridContains scans a race-free rendered snapshot of the grid for
// needle, so a byte-loss bug that shows up anywhere on screen (not just
// a fixed row/column) is caught.
//
// This goes through Render(), never CellAt: vt's SafeEmulator.CellAt
// (charmbracelet/x/vt@v0.0.0-20260816001655/safe_emulator.go:59) takes
// its RLock, calls the embedded Emulator's CellAt, and returns -- but
// what it returns is a *pointer into the emulator's live cell array*,
// dereferenced by the caller (here, cell.Content) only *after* the lock
// has already been released on return. That pointer races any
// concurrent Write to the same grid (e.g. drain's goroutine), and does
// so nondeterministically -- go test -race caught it only on a fraction
// of runs against TestArmingPipeBeforeSeedCaptureDeliversInterstitialBytes,
// not every run (see docs/reports/phase3b-findings.md). SafeEmulator.Render
// (safe_emulator.go:45) holds the RLock across the WHOLE encode and
// returns a plain string, so it is race-free by construction; that is
// the only vt accessor with that property (String() is not safe either
// -- SafeEmulator embeds *Emulator and never overrides String, so calling
// it would call straight through to the unsynchronized encoder).
//
// Render()'s rows are separated by a bare '\n' (charmbracelet/ultraviolet
// Lines.Render), so splitting on that reproduces gridContains' original
// per-row search exactly -- not a single Contains over the whole encoded
// screen, which could let a needle match across an artificial line-wrap
// join that was never contiguous on screen. Each row still needs its
// styling escapes stripped before the search, since Render() only emits
// an SGR/OSC sequence when a cell's style or link actually changes
// (charmbracelet/ultraviolet buffer.go's renderLine), so an escape can
// land in the middle of what was contiguous plain cell content.
func gridContains(g *Grid, needle string) bool {
	for _, row := range strings.Split(g.Render(), "\n") {
		if strings.Contains(ansiEscapeRe.ReplaceAllString(row, ""), needle) {
			return true
		}
	}
	return false
}

// waitFor polls cond until it is true or the deadline passes, returning
// whether it ever became true. Draining a pipe into a grid happens on a
// background goroutine, so assertions on the grid's content need to wait
// for that goroutine to have run, not merely for Start to have returned.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestArmingPipeAfterSeedCaptureLosesInterstitialBytes is the RED control
// PRD II-16 demands: with the pipe-pane armed only after a seed capture
// has already been taken, whatever the pane emits in the gap between the
// two is gone forever -- not in the seed (already captured before the
// emission), and not in the pipe stream (not armed yet when it happened).
func TestArmingPipeAfterSeedCaptureLosesInterstitialBytes(t *testing.T) {
	socket := interactiveSocket("wrong-order")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Wrong order, deliberately: seed capture FIRST.
	seed := rawCapturePane(t, socket, "s0")

	// Something happens on the real pane in the gap -- exactly the race
	// II-16 warns about (a real deployment's gap is scheduling jitter
	// between the two tmux invocations; here it is a keystroke, but the
	// mechanism -- bytes emitted with nobody listening -- is the same).
	sendLiteralLine(t, socket, "s0", "INTERSTITIAL-LOST")

	// Pipe armed only now, strictly after the emission above.
	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	defer pipe.Close()

	grid := newGrid(40, 10)
	if _, err := grid.Write(seed); err != nil {
		t.Fatalf("write seed into grid: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := pipe.Read(buf)
			if n > 0 {
				_, _ = grid.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		_ = pipe.Close()
		<-done
	}()

	// Give the drain goroutine every chance to observe the bytes anyway,
	// so a pass here means "genuinely never delivered", not "raced".
	time.Sleep(300 * time.Millisecond)

	if gridContains(grid, "INTERSTITIAL-LOST") {
		t.Fatalf("grid contains INTERSTITIAL-LOST after arming the pipe AFTER the seed capture; the whole point of this control is that it must NOT be recoverable this way")
	}

	// Demonstrate the loss is real, not a grid-writing bug: the live pane
	// itself still shows it (nothing about the pane's own history was
	// lost, only deck's view of it -- proving the loss is a transport
	// ordering bug, not a rendering one).
	live := rawCapturePane(t, socket, "s0")
	if !strings.Contains(string(live), "INTERSTITIAL-LOST") {
		t.Fatalf("test assumption violated: the live pane itself does not show INTERSTITIAL-LOST; the control proves nothing")
	}
}

// TestArmingPipeBeforeSeedCaptureDeliversInterstitialBytes is II-16's
// positive case: with the pipe armed FIRST, bytes the pane emits between
// arming and the seed capture are queued by tmux and delivered to the
// grid once draining starts, instead of being lost.
func TestArmingPipeBeforeSeedCaptureDeliversInterstitialBytes(t *testing.T) {
	socket := interactiveSocket("right-order")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		// The seed capture happens here, deliberately AFTER Start has
		// already armed the pipe (Start's own contract). Emit the
		// interstitial bytes from inside the seed callback so they land
		// strictly between "pipe armed" and "seed capture taken" on
		// every run, not merely most of the time.
		sendLiteralLine(t, socket, "s0", "INTERSTITIAL-KEPT")
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	if !waitFor(t, 2*time.Second, func() bool { return gridContains(session.Grid(), "INTERSTITIAL-KEPT") }) {
		t.Fatalf("grid never observed INTERSTITIAL-KEPT after arming the pipe BEFORE the seed capture; II-16's whole point is that this ordering must not lose it")
	}
}

// TestExactlyOneGridConstructorCallSite is the mechanical guard for this
// package's own "exactly one grid per selected session" claim: a second
// vt.NewSafeEmulator call site anywhere in this package's non-test source
// would mean a session could end up with a seed written into one emulator
// and a live stream written into another, which is precisely the silent
// divergence PRD II-16/II-42 exists to rule out.
//
// Demonstrate-then-revert: temporarily add a second
// `vt.NewSafeEmulator(1, 1)` call to grid.go and rerun this test -- it goes
// red, naming the file, confirming the check is live and not decorative --
// then remove it again before committing.
// TestGridContainsFindsPlainAndStyledTextButNotAcrossRows is the
// deliberate-mismatch control task 085 (steer 009) asks for alongside
// the Render()-based rewrite: it proves gridContains still finds what it
// used to find (plain text, and text whose row Render() actually breaks
// with a real SGR escape -- a needle that would stop matching if the
// escape-stripping regexp were wrong or missing), and still returns
// false for text that is genuinely absent, INCLUDING a needle built by
// concatenating the tail of one row with the head of the next (which a
// single un-split Contains over the whole rendered screen would
// wrongly match, since the two halves are contiguous once the '\n' row
// separator is gone).
func TestGridContainsFindsPlainAndStyledTextButNotAcrossRows(t *testing.T) {
	g := newGrid(10, 3)

	// Row 0: plain text.
	if _, err := g.Write([]byte("HELLO")); err != nil {
		t.Fatalf("write plain row: %v", err)
	}

	// Row 1: a red-styled word bracketed by plain text on both sides, so
	// Render() must emit an SGR sequence entering AND leaving red style
	// right at the word's boundaries -- exactly the shape that would
	// break a naive (non-escape-stripping) Contains check.
	if _, err := g.Write([]byte("\x1b[2;1HAB\x1b[31mRED\x1b[0mCD")); err != nil {
		t.Fatalf("write styled row: %v", err)
	}

	// Row 2: plain text again, deliberately chosen so its head, glued to
	// row 1's tail, spells a needle that must NOT be found.
	if _, err := g.Write([]byte("\x1b[3;1HXYZ")); err != nil {
		t.Fatalf("write third row: %v", err)
	}

	rendered := g.Render()
	if strings.Contains(rendered, "\x1b") == false {
		t.Fatalf("test assumption violated: Render() emitted no escape sequence at all -- the styled-row case below proves nothing without one; got %q", rendered)
	}

	for _, tc := range []struct {
		needle string
		want   bool
	}{
		{"HELLO", true},        // plain text, unchanged from before the rewrite
		{"ABREDCD", true},      // spans a real SGR escape Render() inserted
		{"RED", true},          // the styled word alone
		{"XYZ", true},          // plain text on the third row
		{"NOPE-ABSENT", false}, // genuinely absent: the deliberate-mismatch control
		{"CDXYZ", false},       // row 1's tail glued to row 2's head: must not match
		{"HELLOAB", false},     // row 0's tail glued to row 1's head: must not match
	} {
		if got := gridContains(g, tc.needle); got != tc.want {
			t.Errorf("gridContains(g, %q) = %v, want %v", tc.needle, got, tc.want)
		}
	}
}

func TestExactlyOneGridConstructorCallSite(t *testing.T) {
	constructorRe := regexp.MustCompile(`vt\.NewSafeEmulator\(`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/interactive): %v", err)
	}

	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		matches := constructorRe.FindAllIndex(data, -1)
		if len(matches) > 0 {
			total += len(matches)
			if len(matches) > 1 {
				t.Errorf("%s: %d vt.NewSafeEmulator call sites in one file, want at most 1 in the whole package", name, len(matches))
			}
		}
	}
	if total != 1 {
		t.Fatalf("package interactive has %d vt.NewSafeEmulator call sites across its non-test source, want exactly 1 (one long-lived grid per session, PRD II-16)", total)
	}
}
