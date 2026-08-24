package tmux

import (
	"context"
	"fmt"
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
func (d *Dispatcher) SendLiteral(ctx context.Context, payload string) error {
	if err := d.Send(ctx, "send-keys", "-l", "--", payload); err != nil {
		return fmt.Errorf("send literal payload to %q: %w", d.target, err)
	}
	return nil
}
