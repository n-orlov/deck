// composed_only_test.go proves PRD phase3b II-26: only COMPOSED cells --
// content plus SGR colour/attribute diffs and OSC-8 hyperlinks, exactly
// what vt.SafeEmulator.Render() (the Grid's own composed-payload method,
// backed by charmbracelet/ultraviolet's Buffer.Render, which never emits
// anything but ansi.ResetStyle/style-diff SGR and hyperlink OSC-8) ever
// produces -- reach the outer terminal. pipe-pane hands deck the pane's
// RAW bytes; a pane is free to switch the alternate screen, reprogram the
// scrolling region, or flip any other terminal-wide DEC private mode, and
// none of that may ever leak into what deck itself emits, because deck's
// own chrome shares that same outer terminal. Passive preview gets this
// property free from capture-pane (it returns cell content and SGR only,
// per task 041/II-19's own finding); the live/pipe path has to reproduce
// it itself by construction, since pipe-pane is not capture-pane.
package interactive

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// dangerousClass is one STRUCTURAL family of escape sequence that would
// corrupt or hijack the OUTER terminal if a raw pane byte stream were
// ever passed through unchanged. Matched by shape (the escape's
// introducer, parameter set and final byte), never by a single named
// literal string, so that e.g. any of the three alternate-screen DEC
// private modes (47/1047/1049) or any of six common mouse-tracking modes
// counts as one occurrence of its class, per the criteria's own "scanned
// for by class not by named string".
type dangerousClass struct {
	name string
	re   *regexp.Regexp
}

var dangerousClasses = []dangerousClass{
	{"alternate-screen switch (DEC private mode 47/1047/1049)", regexp.MustCompile(`\x1b\[\?(?:47|1047|1049)[hl]`)},
	{"scrolling-region programming (DECSTBM)", regexp.MustCompile(`\x1b\[[0-9]*(?:;[0-9]+)?r`)},
	{"origin mode (DECOM, DEC private mode 6)", regexp.MustCompile(`\x1b\[\?6[hl]`)},
	{"mouse tracking enable/disable (DEC private mode 1000/1002/1003/1005/1006/1015)", regexp.MustCompile(`\x1b\[\?10(?:00|02|03|05|06|15)[hl]`)},
	{"bracketed paste enable/disable (DEC private mode 2004)", regexp.MustCompile(`\x1b\[\?2004[hl]`)},
	{"OSC (e.g. window title)", regexp.MustCompile(`\x1b\][0-9]*;[^\x07\x1b]*(?:\x07|\x1b\\)`)},
}

// countDangerous reports, for every class above, how many times it
// occurs in payload.
func countDangerous(payload string) map[string]int {
	counts := make(map[string]int, len(dangerousClasses))
	for _, c := range dangerousClasses {
		counts[c.name] = len(c.re.FindAllStringIndex(payload, -1))
	}
	return counts
}

// dangerousPayload is one printf-able raw byte sequence exercising every
// class above at least once, bracketing a plain, escape-free line of
// visible content that both halves of the test can assert IS present as
// ordinary cell text -- written into the PRIMARY screen, before the
// alternate-screen switch, so it is still the active screen's content by
// the time the payload finishes and leaves the alternate screen again
// (a real full-screen agent's own transcript, from deck's point of view,
// works the same way: primary-screen content survives a visit to the
// alternate screen and back). Sent via runShellPrintf (seed_test.go) so
// tmux's own pty processes it as real PANE OUTPUT, exactly like any
// program's.
const dangerousPayload = `` +
	`HELLO-FROM-PANE\r\n` +
	`\033[?1049h` + // enter the alternate screen
	`\033[3;10r` + // program the scrolling region (DECSTBM)
	`\033[?6h` + // origin mode on
	`\033[?1000h\033[?1006h` + // mouse tracking on (two of the six modes)
	`\033[?2004h` + // bracketed paste on
	`\033]0;evil title\007` + // OSC window-title
	`ALT-SCREEN-CONTENT` +
	`\033[?2004l\033[?1000l\033[?1006l\033[?6l\033[r` + // undo the above
	`\033[?1049l` // leave the alternate screen, back to primary

// TestOnlyComposedCellsReachTheOuterTerminal is PRD II-26's own test: a
// pane emitting alternate-screen switches, scrolling-region programming
// and the other dangerous classes above must never see any of them
// survive into deck's composed payload (Grid.Render()), while the SAME
// bytes, piped straight through with no composition step at all (the raw
// pass-through this requirement rules out), demonstrably DO carry every
// one of those classes -- the mandatory non-vacuous control.
func TestOnlyComposedCellsReachTheOuterTerminal(t *testing.T) {
	socket := interactiveSocket("composed-only")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()

	// --- Control: raw pass-through -----------------------------------
	//
	// A second, independent `pipe-pane -O` (output direction only --
	// the direction a naive raw forwarder would use) captures the SAME
	// pane's raw output bytes to a file with no grid, no emulator, no
	// composition step whatsoever: this is exactly what "raw pane bytes
	// passed through" (PRD II-26's own wording) means. It is armed and
	// disarmed BEFORE deck's own Session ever arms its pipe, since
	// pipe-pane is single-holder-per-pane (task 046/II-24) and the two
	// cannot coexist on the same target.
	rawFile := t.TempDir() + "/raw.out"
	if out, err := exec.Command("tmux", "-L", socket, "pipe-pane", "-O", "-t", "s0", "cat >> "+rawFile).CombinedOutput(); err != nil {
		t.Fatalf("arm raw pipe-pane -O: %v: %s", err, out)
	}
	runShellPrintf(t, socket, "s0", dangerousPayload)
	// Give the pane's process time to actually emit and flush the whole
	// payload before disarming the observer pipe.
	time.Sleep(300 * time.Millisecond)
	if out, err := exec.Command("tmux", "-L", socket, "pipe-pane", "-t", "s0").CombinedOutput(); err != nil {
		t.Fatalf("disarm raw pipe-pane: %v: %s", err, out)
	}
	rawBytes, err := os.ReadFile(rawFile)
	if err != nil {
		t.Fatalf("read raw capture file: %v", err)
	}
	rawCounts := countDangerous(string(rawBytes))
	for _, c := range dangerousClasses {
		if rawCounts[c.name] == 0 {
			t.Fatalf("control invalid: raw pass-through capture contains ZERO occurrences of %q -- the payload never actually drove this class through the real pane, so a green result below would be vacuous", c.name)
		}
	}

	// --- The real thing: deck's own Session --------------------------
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	runShellPrintf(t, socket, "s0", dangerousPayload)
	if !waitFor(t, 3*time.Second, func() bool {
		return gridContains(session.Grid(), "HELLO-FROM-PANE")
	}) {
		t.Fatalf("grid never observed the visible content -- drain may not have processed the payload at all, which would make the zero-occurrences assertion below meaningless")
	}
	// Give the alternate-screen round trip (enter, write, leave) time to
	// finish landing before composing.
	time.Sleep(200 * time.Millisecond)

	composed := session.Grid().Render()
	composedCounts := countDangerous(composed)
	for _, c := range dangerousClasses {
		if got := composedCounts[c.name]; got != 0 {
			t.Errorf("composed payload (Grid.Render()) contains %d occurrence(s) of dangerous class %q -- the raw pass-through control above carried %d occurrence(s) of the SAME class from the SAME bytes, proving Grid.Render() is the only difference", got, c.name, rawCounts[c.name])
		}
	}
}
