package tmux

import (
	"context"
	"fmt"
	"strings"
)

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
		if err := d.Send(ctx, "send-keys", "-l", "--", body); err != nil {
			return fmt.Errorf("send literal payload to %q: %w", d.target, err)
		}
	}
	if semicolons > 0 {
		args := make([]string, 0, semicolons+2)
		args = append(args, "send-keys", "-H")
		for i := 0; i < semicolons; i++ {
			args = append(args, "3b")
		}
		if err := d.Send(ctx, args...); err != nil {
			return fmt.Errorf("send %d peeled trailing semicolon(s) to %q: %w", semicolons, d.target, err)
		}
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
