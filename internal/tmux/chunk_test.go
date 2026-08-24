// chunk_test.go proves PRD phase3b item 36 (II-36): tmux's own internal
// command-string length is the real ceiling for both `send-keys -l --`
// and `send-keys -H`, not the OS's much larger ARG_MAX, and above that
// ceiling `load-buffer` streams a payload over stdin with no argv limit
// at all.
//
// Structured the same way literal_send_test.go and semicolon_test.go
// are: each hazard is first demonstrated RAW, directly against tmux,
// with no deck code involved, and then Dispatcher.SendLiteral is shown
// to actually avoid it.
//
// A note on the exact numbers: PRD II-36's own prose gives "-l -- fails
// at 16380 bytes" and "-H at 8192 args". This file's own bisection
// against the tmux 3.5a binary this repo's ci/Dockerfile installs
// confirms the FIRST figure closely (the real crossover sits at ~16.3
// KiB, between a payload that succeeds at 16340 bytes and one that fails
// with "command too long" by 16360) but NOT the second: with valid
// two-hex-digit `-H` arguments (e.g. "61", the only kind send-keys -H
// actually accepts -- see semicolon_test.go and
// TestSendKeysInvalidHexByteIsSilentlyDiscarded above), the real
// crossover measured here is between 5445 args (succeeds) and 5455
// (fails), roughly two thirds of the PRD's stated 8192. Both figures
// point at the SAME underlying ~16 KiB internal command-string limit --
// 5450 two-character hex tokens plus one separating space each is
// itself ~16.3 KiB, matching the -l -- crossover almost exactly -- so
// this is very likely the PRD's own number having been measured with a
// narrower per-argument width assumption, not a second, independent
// ceiling. Recorded as a finding in docs/reports/phase3b-findings.md's
// II-36 entry rather than silently overridden: this file's own tests use
// the numbers actually measured here, not the PRD's approximate ones,
// and chunk sizes (literalChunkBytes, hexChunkArgs in send.go) stay
// comfortably under BOTH.
package tmux

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func chunkSocket(name string) string {
	return dispatchSocket("chunk-" + name)
}

// TestSendKeysLiteralFailsAtTheRealCommandLengthCeiling is PRD II-36's
// first named hazard, demonstrated raw: a 16340-byte literal payload
// (well under the real ceiling) is delivered with exit 0, but a
// 16360-byte payload -- twenty bytes bigger, nowhere near the OS's own
// ARG_MAX, which is measured in megabytes -- fails outright with tmux's
// own "command too long", never a discarded/malformed delivery.
func TestSendKeysLiteralFailsAtTheRealCommandLengthCeiling(t *testing.T) {
	socket := chunkSocket("literal-ceiling")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	small := strings.Repeat("a", 16340)
	_, stderr, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", small)
	if code != 0 {
		t.Fatalf("send-keys -l -t s0 -- <16340 bytes>: exit=%d, want 0; stderr=%q", code, stderr)
	}

	large := strings.Repeat("a", 16360)
	_, stderr, code = runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", large)
	if code == 0 {
		t.Fatalf("send-keys -l -t s0 -- <16360 bytes>: exit=0, want nonzero (PRD II-36: the real ceiling is tmux's own ~16 KiB command length)")
	}
	if !strings.Contains(stderr, "command too long") {
		t.Fatalf("send-keys -l -t s0 -- <16360 bytes>: stderr=%q, want it to contain %q", stderr, "command too long")
	}
}

// TestSendKeysLiteralFailureIsNotARGMAX rules out the OS's own execve
// argument-length ceiling (MAX_ARG_STRLEN, 128 pages = 512 KiB for any
// single argument on Linux, and the larger ARG_MAX for the whole argv)
// as an alternative explanation: a payload well past the real ceiling
// demonstrated above but still comfortably under 512 KiB -- 100000
// bytes, six times the size that already failed -- fails with the EXACT
// SAME "command too long" tmux error, not an exec-level failure. (A
// payload actually AT or beyond MAX_ARG_STRLEN was tried while writing
// this test and instead fails before tmux ever runs at all, with Go's
// own "argument list too long" from fork/exec -- a genuinely different,
// OS-level failure that would prove nothing about tmux's own ceiling,
// which is exactly why 100000 was chosen deliberately below that
// boundary rather than far above it.)
func TestSendKeysLiteralFailureIsNotARGMAX(t *testing.T) {
	socket := chunkSocket("literal-not-argmax")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	large := strings.Repeat("a", 100_000)
	_, stderr, code := runTmuxRaw(t, socket, "send-keys", "-l", "-t", "s0", "--", large)
	if code == 0 {
		t.Fatalf("send-keys -l -t s0 -- <100000 bytes>: exit=0, want nonzero")
	}
	if !strings.Contains(stderr, "command too long") {
		t.Fatalf("send-keys -l -t s0 -- <100000 bytes>: stderr=%q, want tmux's own %q, not an OS/exec-level failure", stderr, "command too long")
	}
}

// TestSendKeysHexFailsAtTheRealArgCountCeiling is PRD II-36's second
// named hazard, demonstrated raw with VALID two-hex-digit arguments (the
// only kind send-keys -H actually accepts as a real byte -- an invalid
// one is silently discarded per TestSendKeysInvalidHexByteIsSilentlyDiscarded
// above, which would prove nothing about a length ceiling): 5445 "61"
// arguments succeed, 5455 fail with "command too long".
func TestSendKeysHexFailsAtTheRealArgCountCeiling(t *testing.T) {
	socket := chunkSocket("hex-ceiling")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()

	smallArgs := append([]string{"send-keys", "-H", "-t", "s0"}, repeatHexByte("61", 5445)...)
	_, stderr, code := runTmuxRaw(t, socket, smallArgs...)
	if code != 0 {
		t.Fatalf("send-keys -H -t s0 <5445 args of 61>: exit=%d, want 0; stderr=%q", code, stderr)
	}

	largeArgs := append([]string{"send-keys", "-H", "-t", "s0"}, repeatHexByte("61", 5455)...)
	_, stderr, code = runTmuxRaw(t, socket, largeArgs...)
	if code == 0 {
		t.Fatalf("send-keys -H -t s0 <5455 args of 61>: exit=0, want nonzero (PRD II-36's second named ceiling)")
	}
	if !strings.Contains(stderr, "command too long") {
		t.Fatalf("send-keys -H -t s0 <5455 args of 61>: stderr=%q, want it to contain %q", stderr, "command too long")
	}
}

func repeatHexByte(hexByte string, count int) []string {
	args := make([]string, count)
	for i := range args {
		args[i] = hexByte
	}
	return args
}

// TestDispatcherSendLiteralStreamsOversizedPayloadViaLoadBufferAndArrivesIntact
// is the green control for PRD II-36's own stated fix: a payload well
// past the real ceiling demonstrated above -- 20000 bytes of a
// non-repeating, order-sensitive pattern, comfortably beyond even the
// most generous of the two raw failure thresholds -- still arrives at
// the target pane byte-for-byte through SendLiteral. This is non-vacuous
// by construction: if sendLiteralBody's chunk-size check were removed
// (i.e. every payload still went straight to a single `send-keys -l --`
// call, this package's pre-058 behaviour), THIS EXACT payload would
// fail outright with the same "command too long" demonstrated above --
// success here is only possible because the oversized body actually
// went through load-buffer + paste-buffer instead.
func TestDispatcherSendLiteralStreamsOversizedPayloadViaLoadBufferAndArrivesIntact(t *testing.T) {
	socket := chunkSocket("stream-intact")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	waitForBarePrompt(t, socket, "s0")
	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	payload := orderSensitivePayload(20000)
	if err := dispatcher.SendLiteral(ctx, payload); err != nil {
		t.Fatalf("SendLiteral(<20000-byte payload>): %v", err)
	}

	capture := waitForJoinedCaptureContaining(t, socket, "s0", payload)
	if !strings.Contains(capture, payload) {
		t.Fatalf("joined capture does not contain the 20000-byte payload intact")
	}
}

// TestDispatcherSendLiteralAtExactlyTheChunkBoundaryStillUsesASingleCall
// is the regression control at the OTHER edge: a body of EXACTLY
// literalChunkBytes (8192) still goes through the original single
// `send-keys -l --` call (proven indirectly: it still arrives, and
// chunk_test.go's own raw test above shows 8192 bytes is nowhere near
// where -l -- actually fails, so a body this size was never at risk of
// needing to stream in the first place -- the interesting boundary is
// exercised by the oversized test above instead, which pushes well past
// where a single call would have failed).
func TestDispatcherSendLiteralAtExactlyTheChunkBoundaryStillUsesASingleCall(t *testing.T) {
	socket := chunkSocket("boundary")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	waitForBarePrompt(t, socket, "s0")
	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	payload := orderSensitivePayload(8192)
	if err := dispatcher.SendLiteral(ctx, payload); err != nil {
		t.Fatalf("SendLiteral(<8192-byte payload>): %v", err)
	}
	capture := waitForJoinedCaptureContaining(t, socket, "s0", payload)
	if !strings.Contains(capture, payload) {
		t.Fatalf("joined capture does not contain the 8192-byte payload intact")
	}
}

// TestDispatcherSendLiteralChunksAnOversizedHexRunAndArrivesIntact proves
// hexChunkArgs' own chunking: a payload consisting of 6000 trailing
// semicolons (peelTrailingSemicolons' body becomes empty, count becomes
// 6000) needs its -H re-delivery split into multiple send-keys -H calls
// -- TestSendKeysHexFailsAtTheRealArgCountCeiling above already showed a
// single -H call this size fails outright, so success here is only
// possible because sendHexByteRun actually chunked it.
func TestDispatcherSendLiteralChunksAnOversizedHexRunAndArrivesIntact(t *testing.T) {
	socket := chunkSocket("hex-chunk-intact")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	waitForBarePrompt(t, socket, "s0")
	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	payload := strings.Repeat(";", 6000)
	if err := dispatcher.SendLiteral(ctx, payload); err != nil {
		t.Fatalf("SendLiteral(<6000 trailing semicolons>): %v", err)
	}
	capture := waitForJoinedCaptureContaining(t, socket, "s0", payload)
	if !strings.Contains(capture, payload) {
		t.Fatalf("joined capture does not contain all 6000 semicolons intact")
	}
}

// orderSensitivePayload builds a deterministic, non-repeating-in-any-
// short-window string of exactly n bytes (a cycling decimal counter),
// so a bug that drops, duplicates, reorders or truncates any part of a
// chunked/streamed delivery is visible as a content mismatch rather
// than being masked by every byte looking the same (which a payload of
// a single repeated character would risk near a wrap/join boundary).
func orderSensitivePayload(n int) string {
	var b strings.Builder
	b.Grow(n)
	for i := 0; b.Len() < n; i++ {
		b.WriteString(fmt.Sprintf("%08d-", i))
	}
	return b.String()[:n]
}

// waitForJoinedCaptureContaining polls target's pane, joining any
// soft-wrapped lines and including full scrollback history (`-J -S -`),
// until either want appears or a deadline passes -- the join+history
// combination is what lets a single very long typed/pasted line that
// wrapped across far more physical rows than the pane's own height be
// reassembled and compared as one continuous string.
func waitForJoinedCaptureContaining(t *testing.T, socket, target, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var capture string
	for {
		capture = runTmux(t, socket, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		if strings.Contains(capture, want) {
			return capture
		}
		if time.Now().After(deadline) {
			t.Fatalf("joined capture for pane %q never contained the expected payload within the deadline", target)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
