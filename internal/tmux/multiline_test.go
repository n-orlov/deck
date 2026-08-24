// multiline_test.go proves PRD phase3b item 37 (II-37): multi-line
// input -- a payload with at least one embedded "\n" -- must go through
// `load-buffer` + `paste-buffer -d -p`, never a hand-assembled
// `send-keys -H` run with manually inserted ESC[200~/ESC[201~ markers.
//
// Structured the same way chunk_test.go/semicolon_test.go are: the
// hazard is first demonstrated RAW, directly against tmux, with no deck
// code involved (the red control), and then Dispatcher.SendMultiline is
// shown to actually avoid it (the green control), followed by the
// explicit delete-buffer-on-failure proof PRD II-37's own text calls
// out ("`-d` only deletes on success").
//
// Every assertion in this file reads the RECEIVED bytes directly off
// the real filesystem, never through capture-pane: capture-pane renders
// through tmux's own terminal emulation and would report the pane's
// INTERPRETED screen state, which cannot distinguish "\r" from "\n"
// (both just move to the next row) and never shows an escape sequence
// literally at all. The target pane is instead put into raw, no-echo
// mode running `cat > <file>`, so exactly the bytes that arrived at its
// pty -- unmodified by any cooked-mode line-discipline translation --
// land in a file this test's own Go code reads directly.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func multilineSocket(name string) string {
	return dispatchSocket("multiline-" + name)
}

// waitForPromptAtEnd polls target's pane until the LAST non-blank line
// of its current screen is a bare "$" prompt -- unlike
// semicolon_test.go's waitForBarePrompt (which only ever checks line 0,
// suitable for a session that has never had anything typed into it
// yet), this file calls into an already-typed-into pane repeatedly (one
// shell command to enable bracketed-paste mode, a second to arm the raw
// capture), so the prompt this test needs to wait for has long since
// scrolled past row 0.
func waitForPromptAtEnd(t *testing.T, socket, target string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		lines := strings.Split(capture, "\n")
		last := ""
		for i := len(lines) - 1; i >= 0; i-- {
			if trimmed := strings.TrimRight(lines[i], " "); trimmed != "" {
				last = trimmed
				break
			}
		}
		if last == "$" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %q never showed a bare prompt at the end of its screen within the deadline (last capture: %q)", target, capture)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// enableBracketedPasteMode types a shell `printf` command that emits
// ESC[?2004h as PANE OUTPUT (the same technique
// internal/interactive/seed_test.go's runShellPrintf uses for other DEC
// private modes), which is what flips ON tmux's own tracking of
// target's bracketed-paste mode -- the exact condition `paste-buffer -p`
// checks before it will ever emit the ESC[200~/ESC[201~ markers at all.
// TestPasteBufferAddsNoMarkersWithoutBracketedPasteModeEnabled below is
// the raw proof that skipping this step produces NO markers whatsoever,
// which is why every test that asserts on the markers calls this first.
func enableBracketedPasteMode(t *testing.T, socket, target string) {
	t.Helper()
	waitForPromptAtEnd(t, socket, target)
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", `printf '\033[?2004h'`)
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
	time.Sleep(200 * time.Millisecond)
}

// startRawInputCapture types a shell command that puts the pane's own
// pty into raw, no-echo mode and dumps every byte it subsequently reads
// to outFile verbatim. Raw mode (not just -echo) is load-bearing here,
// not just quieter output: cooked mode's ICRNL input translation would
// silently turn any "\r" this file exists to prove arrived back into a
// "\n" before cat's own read(2) ever saw it, which would make the very
// distinction this file tests unobservable.
func startRawInputCapture(t *testing.T, socket, target, outFile string) {
	t.Helper()
	waitForPromptAtEnd(t, socket, target)
	cmd := fmt.Sprintf("stty raw -echo; cat > %s", outFile)
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", cmd)
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
	time.Sleep(200 * time.Millisecond)
}

// waitForFileContaining polls path's content until it contains want or
// a deadline passes, returning the full content either way (so a
// timeout's failure message shows exactly what WAS captured).
func waitForFileContaining(t *testing.T, path, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var data []byte
	for {
		var err error
		data, err = os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q never contained %q within the deadline; got %q", path, want, string(data))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// hexArgsForBytes renders data as the sequence of two-hex-digit
// arguments `send-keys -H` expects, one per byte -- the generic encoder
// TestSendKeysHexWithManualBracketMarkersDeliversTheWrongLineEndings
// needs to hand-craft an entire ESC[200~...ESC[201~-wrapped payload as
// raw -H arguments (sendHexByteRun in send.go only ever repeats a
// single fixed hex byte, which cannot express this).
func hexArgsForBytes(data []byte) []string {
	args := make([]string, len(data))
	for i, b := range data {
		args[i] = fmt.Sprintf("%02x", b)
	}
	return args
}

// TestPasteBufferAddsNoMarkersWithoutBracketedPasteModeEnabled is the
// raw precondition proof enableBracketedPasteMode's own doc comment
// promises: `paste-buffer -p` against a pane whose bracketed-paste mode
// was never turned on adds NO markers at all -- `-p`'s effect is
// conditioned on the target's own mode, exactly like a real attached
// terminal, not an unconditional wrap. (`\n`->`\r` translation still
// happens regardless, since that is paste-buffer's own separate default
// behaviour, off only with `-r`.)
func TestPasteBufferAddsNoMarkersWithoutBracketedPasteModeEnabled(t *testing.T) {
	socket := multilineSocket("no-mode-no-markers")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	outFile := t.TempDir() + "/no-markers.out"
	startRawInputCapture(t, socket, "s0", outFile)

	payload := "line1\nline2\nline3"
	loadFile := writeTempPayload(t, payload)
	runTmux(t, socket, "load-buffer", "-b", "no-mode-buf", loadFile)
	runTmux(t, socket, "paste-buffer", "-d", "-p", "-b", "no-mode-buf", "-t", "s0")

	got := waitForFileContaining(t, outFile, "line3")
	if strings.Contains(got, "\x1b[200~") || strings.Contains(got, "\x1b[201~") {
		t.Fatalf("paste-buffer -p added bracketed-paste markers even though the target's bracketed-paste mode was never enabled: got %q", got)
	}
	want := "line1\rline2\rline3"
	if got != want {
		t.Fatalf("got %q, want %q (\\n->\\r translation is unconditional; only the markers are gated on the mode)", got, want)
	}
}

// writeTempPayload writes payload to a fresh file under t.TempDir() and
// returns its path, for tests that hand a payload to a raw
// `load-buffer <file>` invocation rather than going through
// Dispatcher.SendMultiline (which streams over stdin instead).
func writeTempPayload(t *testing.T, payload string) string {
	t.Helper()
	path := t.TempDir() + "/payload"
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write temp payload: %v", err)
	}
	return path
}

// TestSendKeysHexWithManualBracketMarkersDeliversTheWrongLineEndings is
// PRD II-37's own named red control: a hand-assembled `send-keys -H` run
// carrying literal ESC[200~/ESC[201~ marker bytes around a "\n"-separated
// multi-line body delivers those bytes utterly unmodified -- the
// markers survive, but every "\n" arrives as "\n", never translated to
// "\r" the way a real bracketed paste (or `paste-buffer -d -p`, proven
// below) delivers it. `-H` bypasses tmux's own paste-buffer translation
// entirely; it is a raw hex-byte insertion primitive, not a paste.
func TestSendKeysHexWithManualBracketMarkersDeliversTheWrongLineEndings(t *testing.T) {
	socket := multilineSocket("red-control")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	outFile := t.TempDir() + "/red-control.out"
	startRawInputCapture(t, socket, "s0", outFile)

	payload := "\x1b[200~" + "line1\nline2\nline3" + "\x1b[201~"
	args := append([]string{"send-keys", "-H", "-t", "s0"}, hexArgsForBytes([]byte(payload))...)
	if _, stderr, code := runTmuxRaw(t, socket, args...); code != 0 {
		t.Fatalf("send-keys -H <hand-crafted marker payload>: exit=%d, stderr=%q", code, stderr)
	}

	got := waitForFileContaining(t, outFile, "\x1b[201~")
	if got != payload {
		t.Fatalf("manual -H markers delivered %q, want the UNMODIFIED payload %q -- PRD II-37's own hazard is that -H never translates \\n to \\r", got, payload)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("manual -H markers unexpectedly delivered a \\r byte in %q; this test exists to demonstrate that -H does NOT produce one", got)
	}
}

// TestDispatcherSendMultilineDeliversBracketedPasteWithCRLineEndings is
// the green control: the SAME multi-line body, sent through
// Dispatcher.SendMultiline instead of the red control's hand-assembled
// -H run, arrives wrapped in the real ESC[200~/ESC[201~ markers with
// every "\n" translated to "\r" -- the faithful bracketed-paste
// delivery PRD II-37 requires.
func TestDispatcherSendMultilineDeliversBracketedPasteWithCRLineEndings(t *testing.T) {
	socket := multilineSocket("green-control")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	enableBracketedPasteMode(t, socket, "s0")
	outFile := t.TempDir() + "/green-control.out"
	startRawInputCapture(t, socket, "s0", outFile)

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	payload := "line1\nline2\nline3"
	if err := dispatcher.SendMultiline(ctx, payload); err != nil {
		t.Fatalf("SendMultiline: %v", err)
	}

	want := "\x1b[200~" + "line1\rline2\rline3" + "\x1b[201~"
	got := waitForFileContaining(t, outFile, "\x1b[201~")
	if got != want {
		t.Fatalf("SendMultiline delivered %q, want %q", got, want)
	}
}

// TestDispatcherSendMultilineDeletesTheBufferWhenPasteBufferFails is
// PRD II-37's other named requirement: "-d only deletes on success", so
// the failure path must call delete-buffer explicitly or a buffer is
// left behind forever. load-buffer never touches the target pane at
// all (it only stages bytes server-side), so making target dead BEFORE
// calling SendMultiline still lets load-buffer succeed; it is the
// following paste-buffer step, routed through Dispatcher.Send, that
// then refuses (ErrPaneDead) without ever running the tmux command --
// exactly the failure shape this test needs to exercise the explicit
// delete-buffer call deterministically, with no timing race required.
func TestDispatcherSendMultilineDeletesTheBufferWhenPasteBufferFails(t *testing.T) {
	socket := multilineSocket("failure-deletes-buffer")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	waitForPromptAtEnd(t, socket, "s0")
	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")
	waitForPaneDeadTest(t, client, "s0", 5*time.Second)

	err = dispatcher.SendMultiline(ctx, "line1\nline2")
	if err == nil {
		t.Fatalf("SendMultiline against a dead pane: got nil error, want a refusal")
	}
	if !errors.Is(err, ErrPaneDead) {
		t.Fatalf("SendMultiline against a dead pane: err = %v, want it to wrap ErrPaneDead", err)
	}

	buffers := runTmux(t, socket, "list-buffers")
	if strings.Contains(buffers, "deck-multiline-stream-") {
		t.Fatalf("a multiline-stream buffer was left behind after a failed paste-buffer: list-buffers = %q", buffers)
	}
}
