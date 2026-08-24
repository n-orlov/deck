// semicolon_test.go proves PRD phase3b/phase3c item 33 (II-33): tmux's
// `send-keys -l --` consumes exactly one trailing `;` from a literal
// payload, no matter how many actually trail, EVEN with `--` present;
// interior semicolons are completely safe; and the fix is to peel every
// trailing `;` off up front and re-deliver the peeled count through a
// separate `send-keys -H 3b` call, never through the `\;` escape (PRD
// II-33 states that escape "is not composable").
//
// Structured the same way literal_send_test.go is: each hazard is first
// demonstrated RAW, directly against tmux, with no deck code involved,
// and then Dispatcher.SendLiteral is shown to actually close it.
package tmux

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func semicolonSocket(name string) string {
	return dispatchSocket("semicolon-" + name)
}

// waitForBarePrompt polls target's pane until its shell has actually
// printed its first prompt, before this test sends anything. Without
// this, send-keys can write into the pty before the freshly-forked
// shell has started reading it: the pty's own local echo shows the
// typed text immediately regardless, but the shell's prompt then prints
// itself onto the SAME line, after the already-echoed text instead of
// before it (observed directly: "a;b" typed with zero delay after
// new-session came back as "a;b$" -- the prompt's "$" landing after,
// not before). Every test in this file that asserts an EXACT line, not
// just a substring, needs the race closed at its source rather than
// papered over with a substring check.
func waitForBarePrompt(t *testing.T, socket, target string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		lines := strings.Split(capture, "\n")
		if strings.TrimRight(lines[0], " ") == "$" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %q never showed a bare prompt within the deadline (last capture: %q)", target, capture)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSendKeysConsumesExactlyOneTrailingSemicolonZeroCase is the "zero
// trailing semicolons" control PRD II-33's success criteria names
// explicitly: a payload with no trailing `;` at all is delivered intact,
// nothing peculiar happens just because tmux CAN eat a trailing `;`.
func TestSendKeysConsumesExactlyOneTrailingSemicolonZeroCase(t *testing.T) {
	socket := semicolonSocket("zero")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	waitForBarePrompt(t, socket, "s0")

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", "ab")
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -- ab: exit=%d, want 0", code)
	}
	waitForPaneLine(t, socket, "s0", "$ ab")
}

// TestSendKeysConsumesExactlyOneTrailingSemicolonOneCase is PRD II-33's
// core hazard: a SINGLE trailing `;`, sent bare even with `--` present,
// vanishes completely -- not partially, not left as an error, just gone
// with exit 0.
func TestSendKeysConsumesExactlyOneTrailingSemicolonOneCase(t *testing.T) {
	socket := semicolonSocket("one")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	waitForBarePrompt(t, socket, "s0")

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", "ab;")
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -- ab;: exit=%d, want 0", code)
	}
	waitForPaneLine(t, socket, "s0", "$ ab")
}

// TestSendKeysConsumesExactlyOneTrailingSemicolonTwoCase is PRD II-33's
// "exactly one, not N-1" claim: with TWO trailing semicolons, only ONE
// vanishes -- the other is delivered. If tmux ate every trailing
// semicolon, this would deliver "ab"; if it ate none once `--` is
// present, it would deliver "ab;;". It delivers neither: exactly one is
// lost.
func TestSendKeysConsumesExactlyOneTrailingSemicolonTwoCase(t *testing.T) {
	socket := semicolonSocket("two")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	waitForBarePrompt(t, socket, "s0")

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", "ab;;")
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -- ab;;: exit=%d, want 0", code)
	}
	waitForPaneLine(t, socket, "s0", "$ ab;")
}

// TestSendKeysInteriorSemicolonIsUntouched is PRD II-33's other named
// claim: a semicolon that is NOT at the very end of the payload is
// completely safe and needs no special handling at all.
func TestSendKeysInteriorSemicolonIsUntouched(t *testing.T) {
	socket := semicolonSocket("interior")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	waitForBarePrompt(t, socket, "s0")

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", "a;b")
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -- a;b: exit=%d, want 0", code)
	}
	waitForPaneLine(t, socket, "s0", "$ a;b")
}

// TestDispatcherSendLiteralDeliversZeroTrailingSemicolons is the green
// control matching the zero-case raw hazard above: nothing changes for
// a payload with no trailing ';' at all.
func TestDispatcherSendLiteralDeliversZeroTrailingSemicolons(t *testing.T) {
	assertSendLiteralDelivers(t, "zero-green", "ab", "ab")
}

// TestDispatcherSendLiteralDeliversOneTrailingSemicolon is the green
// control for PRD II-33's core hazard: through SendLiteral, a single
// trailing ';' that would otherwise vanish is delivered intact.
func TestDispatcherSendLiteralDeliversOneTrailingSemicolon(t *testing.T) {
	assertSendLiteralDelivers(t, "one-green", "ab;", "ab;")
}

// TestDispatcherSendLiteralDeliversTwoTrailingSemicolons is the green
// control for the two-trailing-semicolon case: BOTH must survive, not
// just the one tmux would have left behind on its own.
func TestDispatcherSendLiteralDeliversTwoTrailingSemicolons(t *testing.T) {
	assertSendLiteralDelivers(t, "two-green", "ab;;", "ab;;")
}

// TestDispatcherSendLiteralDeliversThreeTrailingSemicolons goes one
// beyond the PRD's named zero/one/two cases to confirm
// peelTrailingSemicolons' fixed-point loop (not merely a single peel)
// is what is actually wired in -- a single-peel implementation would
// still lose exactly one semicolon here (the resent remainder would
// itself end in ';' and tmux would eat it again).
func TestDispatcherSendLiteralDeliversThreeTrailingSemicolons(t *testing.T) {
	assertSendLiteralDelivers(t, "three-green", "ab;;;", "ab;;;")
}

// TestDispatcherSendLiteralLeavesInteriorSemicolonUntouched is the green
// control for the interior-semicolon claim, combined with a trailing one
// so both code paths (the untouched body and the peeled tail) are
// exercised in the same payload.
func TestDispatcherSendLiteralLeavesInteriorSemicolonUntouched(t *testing.T) {
	assertSendLiteralDelivers(t, "interior-and-trailing-green", "a;b;", "a;b;")
}

// TestDispatcherSendLiteralDeliversAllSemicolonPayload covers the
// degenerate case where the whole payload is nothing but trailing
// semicolons -- peelTrailingSemicolons' body is empty, so SendLiteral
// must skip the now-pointless empty `-l --` call and rely solely on the
// `-H` batch.
func TestDispatcherSendLiteralDeliversAllSemicolonPayload(t *testing.T) {
	assertSendLiteralDelivers(t, "all-semicolon-green", ";;", ";;")
}

func assertSendLiteralDelivers(t *testing.T, name, payload, wantSuffix string) {
	t.Helper()
	socket := semicolonSocket(name)
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	waitForBarePrompt(t, socket, "s0")
	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := dispatcher.SendLiteral(ctx, payload); err != nil {
		t.Fatalf("SendLiteral(%q): %v", payload, err)
	}
	waitForPaneLine(t, socket, "s0", "$ "+wantSuffix)
}

// waitForPaneLine polls target's pane-0 capture until its FIRST line
// (trimmed of trailing blank rows capture-pane pads the output with)
// equals want, or fails after a timeout. A single immediate capture
// right after send-keys returns is not reliable -- the pty's echo of a
// just-delivered keystroke can still be in flight when tmux's own
// send-keys call returns, so this package's other raw-hazard checks
// that assert an EXACT final line (not just a substring) need the same
// poll-until-stable discipline waitForLiteralSendMarker already uses for
// substring checks.
func waitForPaneLine(t *testing.T, socket, target, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		lines := strings.Split(capture, "\n")
		firstLine := strings.TrimRight(lines[0], " ")
		if firstLine == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %q first line = %q, want %q (full capture: %q)", target, firstLine, want, capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestPeelTrailingSemicolonsIsAFixedPointLoop is a direct unit test of
// peelTrailingSemicolons itself, independent of tmux, covering the
// exact zero/one/two cases PRD II-33 names plus the three-and-all-
// semicolon cases the Dispatcher-level tests above add.
func TestPeelTrailingSemicolonsIsAFixedPointLoop(t *testing.T) {
	cases := []struct {
		payload   string
		wantBody  string
		wantCount int
	}{
		{"ab", "ab", 0},
		{"ab;", "ab", 1},
		{"ab;;", "ab", 2},
		{"ab;;;", "ab", 3},
		{"a;b", "a;b", 0},
		{"a;b;", "a;b", 1},
		{"", "", 0},
		{";", "", 1},
		{";;", "", 2},
	}
	for _, c := range cases {
		body, count := peelTrailingSemicolons(c.payload)
		if body != c.wantBody || count != c.wantCount {
			t.Fatalf("peelTrailingSemicolons(%q) = (%q, %d), want (%q, %d)", c.payload, body, count, c.wantBody, c.wantCount)
		}
	}
}

// backslashSemicolonEscape is what
// TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolons hunts for:
// the literal two-character sequence `\;` (a backslash immediately
// followed by a semicolon) anywhere in this package's production code.
// PRD II-33 states this tmux escape "is not composable" with the rest
// of this package's dispatch machinery, so peelTrailingSemicolons'
// strip-and-re-encode-as--H approach is the only form permitted.
var backslashSemicolonEscape = regexp.MustCompile(`\\;`)

// TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolons is the grep
// proof PRD II-33's success criteria names directly: no production .go
// file in this package ever uses tmux's `\;` escape.
func TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolons(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if backslashSemicolonEscape.MatchString(line) {
				t.Fatalf("%s:%d: uses tmux's \\; escape: %q -- PRD II-33 says this escape is not composable, use peelTrailingSemicolons + -H instead", name, i+1, trimmed)
			}
		}
	}
}

// TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolonsIsNonVacuous
// proves the guard above actually fires against a realistic violation,
// the same demonstrate-then-revert discipline this package's other grep
// guards use.
func TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolonsIsNonVacuous(t *testing.T) {
	violation := `	if err := d.Send(ctx, "send-keys", "-l", "--", payload+"\\;"); err != nil {`
	if !backslashSemicolonEscape.MatchString(violation) {
		t.Fatalf("the guard's own pattern does not match a realistic \\; violation -- it would not have caught it: %q", violation)
	}
}
