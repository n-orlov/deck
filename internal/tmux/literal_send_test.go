// literal_send_test.go proves PRD phase3c items 32 and 38 (II-32/II-38):
// every literal payload deck sends must go through `send-keys -l --`,
// and every tmux invocation's exit status must actually be checked.
//
// Each hazard below is first demonstrated RAW, directly against tmux,
// with no deck code involved at all -- proving the hazard is real, not
// a property of deck's own code -- and then, where deck has a primitive
// that should avoid it (Dispatcher.SendLiteral, send.go), a green
// control shows the primitive actually avoids it.
package tmux

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func literalSendSocket(name string) string {
	return dispatchSocket("literal-" + name)
}

// TestSendKeysDashPayloadWithoutSeparatorIsSilentlyDiscarded is PRD
// II-32's first named hazard: without `--`, a payload beginning with `-`
// is silently discarded with exit 0. The payload used here is literally
// "-l" (not "-foo") deliberately -- it is one of send-keys' OWN flag
// letters, so tmux's getopt-style parser keeps consuming it as ANOTHER
// occurrence of the -l flag rather than ever treating it as a positional
// payload; nothing is left to type. This is the sharpest form of the
// hazard: checking the tmux exit code cannot tell "delivered" apart from
// "swallowed as a flag" here, because both report success.
func TestSendKeysDashPayloadWithoutSeparatorIsSilentlyDiscarded(t *testing.T) {
	socket := literalSendSocket("dash-no-sep")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	stdout, stderr, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "-l")
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -l (no --): exit=%d, want 0 (PRD II-32: this hazard is SILENT); stdout=%q stderr=%q", code, stdout, stderr)
	}
	capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
	if strings.Contains(capture, "-l") {
		t.Fatalf("pane capture = %q, contains the payload \"-l\" -- want it discarded (that is the hazard)", capture)
	}
}

// TestSendKeysDoubleDashHelpWithoutSeparatorErrors is PRD II-32's second
// named hazard: a payload of exactly "--help", sent without `--`, is a
// DIFFERENT failure shape from the general dash-prefixed case above --
// tmux parses it as its own (invalid) long-flag syntax and errors
// outright, rather than silently discarding it. Both are wrong; neither
// is safe to rely on for detecting the other.
func TestSendKeysDoubleDashHelpWithoutSeparatorErrors(t *testing.T) {
	socket := literalSendSocket("dashdash-help")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	_, stderr, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--help")
	if code == 0 {
		t.Fatalf("send-keys -l -t s0 --help (no --): exit=0, want nonzero -- PRD II-32 says this form errors")
	}
	if stderr == "" {
		t.Fatalf("send-keys -l -t s0 --help (no --): stderr empty, want tmux's own flag-parsing error message")
	}
}

// TestSendKeysWithoutLiteralFlagEnterBecomesACarriageReturn is PRD
// II-32's third named hazard: without `-l`, tmux treats the four-letter
// string "Enter" as a KEY NAME (a real Enter keypress), not as four
// characters of text. Proven against an otherwise-empty prompt line: a
// literal-text "Enter" would leave the row reading "Enter" followed by
// tmux's own trailing blanks; a real Enter keypress instead advances the
// cursor to a new, still-empty prompt line, with no "Enter" text
// anywhere in the pane.
func TestSendKeysWithoutLiteralFlagEnterBecomesACarriageReturn(t *testing.T) {
	socket := literalSendSocket("enter-cr")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-t", "s0", "--", "Enter")
	if code != 0 {
		t.Fatalf("send-keys -t s0 -- Enter (no -l): exit=%d, want 0", code)
	}
	time.Sleep(200 * time.Millisecond)
	capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
	if strings.Contains(capture, "Enter") {
		t.Fatalf("pane capture = %q, contains the literal text \"Enter\" -- want a bare carriage return instead (PRD II-32)", capture)
	}
	lines := strings.Split(capture, "\n")
	if len(lines) < 2 {
		t.Fatalf("pane capture has only %d line(s), want at least 2 (the carriage return should have advanced the cursor to a new prompt line): %q", len(lines), capture)
	}
}

// TestSendKeysDoubledLiteralFlagWithoutSeparatorIsSilentlyDiscarded is
// PRD II-38's first named "returns 0" hazard: an accidentally doubled
// `-l` immediately followed by a dash-shaped payload with no `--`
// separator hits the exact same swallowed-as-a-flag mechanism as
// TestSendKeysDashPayloadWithoutSeparatorIsSilentlyDiscarded above --
// named separately in the PRD because a caller might introduce the
// doubled flag by accident (e.g. two code paths each adding their own
// `-l`) without ever intending the missing `--`.
func TestSendKeysDoubledLiteralFlagWithoutSeparatorIsSilentlyDiscarded(t *testing.T) {
	socket := literalSendSocket("doubled-l")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	_, _, code := runTmuxRaw(t, socket, "send-keys", "-l", "-l", "-t", "s0", "-l")
	if code != 0 {
		t.Fatalf("send-keys -l -l -t s0 -l (no --): exit=%d, want 0", code)
	}
	capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
	if strings.Contains(capture, "-l") {
		t.Fatalf("pane capture = %q, contains the payload \"-l\" -- want it discarded (PRD II-38)", capture)
	}
}

// TestSendKeysInvalidHexByteIsSilentlyDiscarded is PRD II-38's second
// named "returns 0" hazard: `-H zz` (zz is not a valid hex byte) is
// accepted by tmux with exit 0 and delivers nothing.
func TestSendKeysInvalidHexByteIsSilentlyDiscarded(t *testing.T) {
	socket := literalSendSocket("hex-zz")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	_, stderr, code := runTmuxRaw(t, socket, "send-keys", "-H", "zz", "-t", "s0")
	if code != 0 {
		t.Fatalf("send-keys -H zz -t s0: exit=%d, want 0; stderr=%q", code, stderr)
	}
	capture := strings.TrimSpace(runTmux(t, socket, "capture-pane", "-p", "-t", "s0"))
	if capture != "$" {
		t.Fatalf("pane capture = %q, want only the bare prompt (nothing delivered) -- PRD II-38", capture)
	}
}

// TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero is PRD
// II-38's third named "returns 0" hazard, and PRD item 35's own hazard
// under its own name: an unrecognized key name is not rejected -- it is
// typed into the target AS LITERAL TEXT, character by character, with
// exit 0 and no error at all. This is the opposite failure shape from
// the two above (which discard); grouping all three under "returns 0"
// is exactly the point PRD II-38 makes: the exit code alone cannot tell
// any of "delivered correctly", "silently discarded" and "delivered
// WRONG" apart.
func TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero(t *testing.T) {
	socket := literalSendSocket("unknown-key")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	_, stderr, code := runTmuxRaw(t, socket, "send-keys", "-t", "s0", "Frobnicate")
	if code != 0 {
		t.Fatalf("send-keys -t s0 Frobnicate: exit=%d, want 0; stderr=%q", code, stderr)
	}
	capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
	if !strings.Contains(capture, "Frobnicate") {
		t.Fatalf("pane capture = %q, want it to contain the literal text \"Frobnicate\" (ten bytes typed as-is, PRD item 35/II-38)", capture)
	}
}

// TestDispatcherSendLiteralDeliversADashPrefixedPayloadCorrectly is the
// green control for the first two hazards: Dispatcher.SendLiteral's
// fixed `-l --` form delivers a payload that would otherwise be silently
// discarded (a bare "-l") or would otherwise error ("--help") -- both
// land in the pane as exactly the bytes given, proving the mitigation
// actually closes the raw hazards demonstrated above, not merely avoids
// re-deriving them.
func TestDispatcherSendLiteralDeliversADashPrefixedPayloadCorrectly(t *testing.T) {
	socket := literalSendSocket("green-dash")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	if err := dispatcher.SendLiteral(ctx, "-l"); err != nil {
		t.Fatalf("SendLiteral(-l): %v", err)
	}
	waitForLiteralSendMarker(t, socket, "s0", "-l")
}

// TestDispatcherSendLiteralDeliversDoubleDashHelpCorrectly is the green
// control for PRD II-32's "--help errors" hazard: through SendLiteral,
// the exact same payload that errors when sent bare (task
// TestSendKeysDoubleDashHelpWithoutSeparatorErrors above) is delivered
// as plain text with no error at all.
func TestDispatcherSendLiteralDeliversDoubleDashHelpCorrectly(t *testing.T) {
	socket := literalSendSocket("green-help")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	if err := dispatcher.SendLiteral(ctx, "--help"); err != nil {
		t.Fatalf("SendLiteral(--help): %v", err)
	}
	waitForLiteralSendMarker(t, socket, "s0", "--help")
}

func waitForLiteralSendMarker(t *testing.T, socket, target, marker string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		if strings.Contains(capture, marker) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker %q never appeared in pane %q output:\n%s", marker, target, capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// literalFlagWithoutSeparator is what
// TestEveryLiteralSendUsesDashLDashDash hunts for: a "-l" flag appearing
// on the same line as "send-keys" without a "--" argument separator also
// appearing on that line. Every literal send this package performs
// builds its whole argv on one line (send.go's SendLiteral is the one
// call site as of this task), so a single-line check is sufficient and
// deliberately simple -- a future call site that splits its argv across
// lines to dodge this check would itself be worth a second look.
var sendKeysWithDashL = regexp.MustCompile(`"send-keys"[^\n]*"-l"|"-l"[^\n]*"send-keys"`)

// TestEveryLiteralSendUsesDashLDashDash is the grep proof PRD II-32's
// success criteria names directly: no production .go file in this
// package ever builds a `send-keys -l` invocation without `--` also
// present on the same line.
func TestEveryLiteralSendUsesDashLDashDash(t *testing.T) {
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
			if sendKeysWithDashL.MatchString(line) && !strings.Contains(line, `"--"`) {
				t.Fatalf("%s:%d: send-keys -l without -- on the same line: %q -- PRD II-32 requires -l and -- together for every literal send", name, i+1, trimmed)
			}
		}
	}
}

// TestEveryLiteralSendUsesDashLDashDashIsNonVacuous proves the guard
// above actually fires against a realistic violation (the doubled-flag
// hazard proven raw above), the same demonstrate-then-revert discipline
// this package's other grep guards use.
func TestEveryLiteralSendUsesDashLDashDashIsNonVacuous(t *testing.T) {
	violation := `	if _, err := d.client.run(ctx, "send-keys", "-l", "-t", d.target, payload); err != nil {`
	if !sendKeysWithDashL.MatchString(violation) {
		t.Fatalf("the guard's own pattern does not match a realistic -l-without-- violation -- it would not have caught it")
	}
	if strings.Contains(violation, `"--"`) {
		t.Fatalf("test setup invalid: the violation string itself contains \"--\"")
	}
}

// dangerousDispatchWithDiscardedError is what
// TestNoIgnoredExitStatusForInputDispatch hunts for: any of the three
// input-dispatch commands (PRD II-30's list) whose return value is
// discarded via Go's blank identifier instead of being checked. PRD
// II-38's whole point is that a nonzero tmux exit is sometimes the ONLY
// signal something went wrong (unlike the three exit-0 hazards above,
// which need a structural fix instead) -- discarding it anywhere would
// silently reopen exactly that. The discard prefix covers both shapes
// this package actually has: `_, _ = ...` for a two-value Client.run
// call, and a bare `_ = ...` for a single-value Dispatcher.Send/
// SendLiteral call.
var dangerousDispatchDiscardPrefix = regexp.MustCompile(`^_\s*(,\s*_\s*)?=[^=]`)
var dangerousDispatchCommand = regexp.MustCompile(`"(send-keys|paste-buffer|load-buffer)"`)

// TestNoIgnoredExitStatusForInputDispatch is the grep proof PRD II-38's
// success criteria names directly: no production .go file in this
// package ever discards the error from a send-keys/paste-buffer/
// load-buffer invocation. This is deliberately narrower than "every
// tmux call's error is checked" -- pipe.go's own disarm command (a
// DIFFERENT command, "pipe-pane", not one of these three) discards its
// error there on purpose and is documented at the call site as to why;
// this guard is scoped to the commands that can deliver INPUT to a live
// pane, which is what PRD II-32/II-38 are actually about.
func TestNoIgnoredExitStatusForInputDispatch(t *testing.T) {
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
			if dangerousDispatchDiscardPrefix.MatchString(trimmed) && dangerousDispatchCommand.MatchString(line) {
				t.Fatalf("%s:%d: input-dispatch command's error discarded: %q -- PRD II-38 requires every tmux exit status to be checked", name, i+1, trimmed)
			}
		}
	}
}

// TestNoIgnoredExitStatusForInputDispatchIsNonVacuous proves the guard
// above actually fires against a realistic violation.
func TestNoIgnoredExitStatusForInputDispatchIsNonVacuous(t *testing.T) {
	twoValue := `_, _ = d.client.run(ctx, "send-keys", "-l", "--", payload)`
	if !dangerousDispatchDiscardPrefix.MatchString(twoValue) || !dangerousDispatchCommand.MatchString(twoValue) {
		t.Fatalf("the guard's own pattern does not match a realistic two-value discarded-error violation -- it would not have caught it: %q", twoValue)
	}
	singleValue := `_ = d.Send(ctx, "send-keys", "-l", "--", payload)`
	if !dangerousDispatchDiscardPrefix.MatchString(singleValue) || !dangerousDispatchCommand.MatchString(singleValue) {
		t.Fatalf("the guard's combined check does not match a realistic single-value discarded-error violation -- it would not have caught it: %q", singleValue)
	}
}
