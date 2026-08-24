package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// literalChunkBytes and hexChunkArgs are PRD II-36's two chunk sizes.
// Both are deliberately well under half of the real, empirically
// measured ceilings recorded in chunk_test.go's raw hazard demonstrations
// (TestSendKeysLiteralFailsAtTheRealCommandLengthCeiling,
// TestSendKeysHexFailsAtTheRealArgCountCeiling): tmux's own internal
// command-string representation, not the OS's ARG_MAX (which is measured
// in megabytes, not kilobytes -- a payload orders of magnitude below
// ARG_MAX still fails against tmux itself). PRD II-36's own prose gives
// approximate figures ("-l -- fails at 16380 bytes", "-H at 8192 args");
// this package's own measurement (chunk_test.go, tmux 3.5a) puts the
// real crossover closer to ~16.3 KiB for -l -- (consistent with the
// PRD) and ~5.4-5.5 thousand valid two-hex-digit -H arguments (lower
// than the PRD's 8192 -- see docs/reports/phase3b-findings.md's II-36
// entry for the reproduction and the corrected number). Both chunk
// sizes below stay comfortably under EITHER figure.
const (
	// literalChunkBytes is the largest literal body SendLiteral will ever
	// hand to a single `send-keys -l --` call. A body longer than this is
	// streamed through load-buffer + paste-buffer instead (see
	// streamLiteralViaLoadBuffer) -- one tmux round trip regardless of
	// size, rather than many chunked send-keys calls, since load-buffer
	// reads its payload over stdin with no argv-length ceiling at all.
	literalChunkBytes = 8192
	// hexChunkArgs is the largest number of `-H` hex-byte arguments
	// SendLiteral will ever hand to a single send-keys -H call. -H has no
	// load-buffer equivalent (load-buffer/paste-buffer inserts raw literal
	// bytes, which is exactly the -l parser hazard -H exists to route
	// around for peeled trailing semicolons -- semicolon_test.go), so an
	// oversized -H send is chunked into multiple calls instead of streamed.
	hexChunkArgs = 4096
)

// literalStreamSeq is a package-level counter that makes every buffer
// name streamLiteralViaLoadBuffer picks unique for the life of this
// process, even across concurrent Dispatchers on the same tmux server --
// two overlapping oversized sends must never race over the same buffer
// name and risk one paste-buffer -d deleting the other's still-unread
// payload.
var literalStreamSeq int64

func nextLiteralBufferName() string {
	n := atomic.AddInt64(&literalStreamSeq, 1)
	return fmt.Sprintf("deck-literal-stream-%d-%d", os.Getpid(), n)
}

// SendLiteral dispatches payload as literal typed text via
// `send-keys -l --`, the ONLY form PRD phase3c item 32 (II-32) permits
// for a literal payload -- and does so through Dispatcher.Send, so the
// verified pane id captured at entry is always the real target (PRD
// II-30) and the command's own tmux exit is always checked (PRD II-38).
//
// Both flags are mandatory and neither substitutes for the other:
//
//   - `--` is what makes a payload beginning with "-" safe. Without it,
//     tmux's own getopt-style argument parser keeps interpreting a
//     leading-dash payload as MORE flags to send-keys itself: a payload
//     that happens to coincide with one of send-keys' own flag letters
//     (e.g. literally "-l") is silently swallowed as a duplicate flag and
//     NOTHING is delivered, exit 0, no error -- and a payload that does
//     not name a flag send-keys recognizes (e.g. "-foo") instead fails
//     outright with "unknown flag". Neither behaviour is what the caller
//     asked for, and the failure mode differs by payload, which is
//     exactly why grepping for the presence of `--` on every literal
//     send is the only reliable guard -- checking the tmux exit code
//     cannot distinguish "safely delivered" from "silently swallowed"
//     for a payload that happens to look like one of send-keys' own
//     flags. A payload that happens to equal EXACTLY "--help" is a
//     further special case: without `--`, tmux parses it as ITS OWN
//     `--help`-shaped invalid-flag sequence and errors outright, rather
//     than swallowing it silently -- still wrong, just a different wrong
//     (see literal_send_test.go's TestSendKeysDoubleDashHelpWithoutSeparatorErrors).
//   - `-l` is what makes the payload literal text at all. Without it,
//     tmux treats every word of the payload as a KEY NAME to translate,
//     not as characters to type: the four-character string "Enter" would
//     become a single carriage return, not those four letters (see
//     literal_send_test.go's TestSendKeysWithoutLiteralFlagEnterBecomesACarriageReturn).
//
// PRD II-38 separately names three tmux invocations that all report
// success (exit 0) despite being wrong in some way -- checking the exit
// code alone would never catch any of them, which is why SendLiteral's
// own argv is fixed and never caller-assembled: `-l -l` (an accidentally
// doubled `-l` immediately followed by a payload that itself looks like
// a flag, without `--`, silently delivers nothing -- the exact hazard
// `--` above closes), `-H zz` (an invalid hex byte to send-keys' -H mode,
// silently delivers nothing), and an unrecognized key name (delivered
// into the target as literal text, exit 0 -- PRD item 35's hazard,
// closed by never reaching this function with anything but -l --
// literal text in the first place).
//
// PRD II-33 is a THIRD hazard, orthogonal to both of the above: even
// with `--` present, tmux's `-l` parser consumes exactly ONE trailing
// `;` from the payload, no matter how many actually trail. This is not
// a guess -- semicolon_test.go's raw (no-deck-code) tests prove it
// directly: a bare `send-keys -l -- "ab;"` delivers "ab" (the lone
// trailing `;` vanishes); `"ab;;"` delivers "ab;" (one of the two
// vanishes, never both); `"ab;;;"` delivers "ab;;". Always exactly one
// fewer `;` than actually trailed -- never zero fewer, never more than
// one fewer -- and interior semicolons (anywhere but the very end) are
// completely unaffected either way. SendLiteral closes this by calling
// peelTrailingSemicolons first: every trailing `;` is stripped from the
// payload up front (so the remainder has nothing left at its end for
// tmux's parser to eat and cannot lose anything), and every peeled `;`
// is re-delivered explicitly via a SEPARATE `send-keys -H 3b` call
// (one hex byte per peeled semicolon, batched into one invocation) --
// `-H` bypasses the `-l` literal-text parser entirely, so a hex-encoded
// semicolon can never be reinterpreted as tmux's own trailing-`;`
// command separator. The PRD's own `\;` escape is deliberately NOT used:
// PRD II-33 states it "is not composable" with the rest of this
// package's dispatch machinery, and semicolon_test.go's
// TestNoBackslashSemicolonEscapeIsUsedForTrailingSemicolons greps this
// package to prove it never falls back to it.
func (d *Dispatcher) SendLiteral(ctx context.Context, payload string) error {
	body, semicolons := peelTrailingSemicolons(payload)
	if body != "" || semicolons == 0 {
		if err := d.sendLiteralBody(ctx, body); err != nil {
			return err
		}
	}
	if semicolons > 0 {
		if err := d.sendHexByteRun(ctx, semicolons, "3b"); err != nil {
			return fmt.Errorf("send %d peeled trailing semicolon(s) to %q: %w", semicolons, d.target, err)
		}
	}
	return nil
}

// sendLiteralBody is SendLiteral's own split point for PRD II-36: a body
// at or under literalChunkBytes goes through the single `send-keys -l --`
// call this package always used (unchanged for every payload any
// pre-existing test already covers); a larger body is streamed through
// load-buffer + paste-buffer instead of ever being handed to send-keys's
// own argv at all, so it can never approach the real ~16 KiB command-
// string ceiling chunk_test.go measures, no matter how large it is.
func (d *Dispatcher) sendLiteralBody(ctx context.Context, body string) error {
	if len(body) <= literalChunkBytes {
		if err := d.Send(ctx, "send-keys", "-l", "--", body); err != nil {
			return fmt.Errorf("send literal payload to %q: %w", d.target, err)
		}
		return nil
	}
	return d.streamLiteralViaLoadBuffer(ctx, body)
}

// streamLiteralViaLoadBuffer delivers an oversized literal body via
// `load-buffer` (reading body over stdin, so its size is never bounded by
// any argv length) followed by `paste-buffer -d`, which both inserts the
// buffer's raw bytes into the target pane and -- per task 058/II-37's own
// finding, restated here because this call site predates that task's
// multi-line handling -- only deletes the buffer on SUCCESS: a failed
// paste-buffer leaves the loaded buffer behind unless deleted explicitly,
// which is why the failure path below calls delete-buffer itself. The
// load-buffer step does not go through Dispatcher.Send: it never touches
// the target pane (it only stages bytes into a server-side buffer), so
// there is nothing for Send's identity re-verification to protect against
// yet -- that happens at the paste-buffer step, which does go through
// Send like every other actual delivery in this package.
func (d *Dispatcher) streamLiteralViaLoadBuffer(ctx context.Context, body string) error {
	name := nextLiteralBufferName()
	if _, err := d.client.runWithStdin(ctx, strings.NewReader(body), "load-buffer", "-b", name, "-"); err != nil {
		return fmt.Errorf("stream oversized literal payload (%d bytes) to %q via load-buffer: %w", len(body), d.target, err)
	}
	if err := d.Send(ctx, "paste-buffer", "-d", "-b", name); err != nil {
		if _, delErr := d.client.run(ctx, "delete-buffer", "-b", name); delErr != nil {
			return fmt.Errorf("paste streamed literal payload (%d bytes) to %q: %w (buffer %q also left behind: delete-buffer failed: %v)", len(body), d.target, err, name, delErr)
		}
		return fmt.Errorf("paste streamed literal payload (%d bytes) to %q: %w", len(body), d.target, err)
	}
	return nil
}

// sendHexByteRun re-delivers count copies of the single hex byte
// hexByte (semicolon_test.go's peeled-trailing-semicolon use is the only
// caller today) via one or more `send-keys -H` calls, never more than
// hexChunkArgs hex arguments in any single call -- PRD II-36's second
// named ceiling. There is no load-buffer substitute for this path (see
// hexChunkArgs's own doc comment above), so an oversized run is chunked
// into multiple calls instead of streamed.
func (d *Dispatcher) sendHexByteRun(ctx context.Context, count int, hexByte string) error {
	remaining := count
	for remaining > 0 {
		chunk := remaining
		if chunk > hexChunkArgs {
			chunk = hexChunkArgs
		}
		args := make([]string, 0, chunk+2)
		args = append(args, "send-keys", "-H")
		for i := 0; i < chunk; i++ {
			args = append(args, hexByte)
		}
		if err := d.Send(ctx, args...); err != nil {
			return err
		}
		remaining -= chunk
	}
	return nil
}

// peelTrailingSemicolons strips every trailing `;` off payload's end
// (never an interior one -- the loop only ever inspects the current
// last byte, so it stops the instant that byte is not `;`) and reports
// how many it removed. Peeling ALL of them, not just one, is what makes
// SendLiteral correct for any trailing count: PRD II-33's hazard loses
// exactly one `;` per literal send regardless of how many trail, so a
// naive single-peel-then-resend would still lose one whenever two or
// more trail (the resent remainder would itself end in `;` and tmux
// would eat it again). Peeling to a fixed point first, then sending the
// now-semicolon-free body once and re-delivering the peeled count via
// `-H` in one batched call, has nothing left for tmux's `-l` parser to
// eat at any step.
func peelTrailingSemicolons(payload string) (body string, count int) {
	body = payload
	for strings.HasSuffix(body, ";") {
		body = body[:len(body)-1]
		count++
	}
	return body, count
}
