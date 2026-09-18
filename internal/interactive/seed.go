package interactive

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/tmux"
)

// BuildSeed assembles the exact byte stream PRD phase3b II-17/II-18
// require to seed a FRESH grid (task 044/II-22 owns making every reseed
// use a fresh parser; this function's caller is what supplies that) into
// the same state a live tmux pane was in when state and body were
// captured.
//
// The order below is load-bearing, not stylistic (prds/phase3b-interactive-preview.md
// #17-18):
//
//  1. alternate-screen selection FIRST, from state.AlternateOn -- 1049
//     clears the buffer it switches TO, so anything written before this
//     step could be discarded by the switch, and anything written after
//     inherits whichever buffer this step leaves active.
//  2. a neutral painting state (origin off, full-width scroll region,
//     wrap on, insert off) so the capture body below paints exactly as
//     tmux intended it, uncontaminated by whatever the pane's REAL final
//     mode state (steps 4-7) will turn out to be.
//  3. the capture body VERBATIM -- never re-addressed or SGR-reset per
//     line. `capture-pane -e` is one continuous SGR stream across every
//     row: a real capture's row N can begin with a bare glyph and no SGR
//     of its own at all, inheriting the pen from row N-1's last cell. A
//     seed that emits `ESC[<row>;1H ESC[0m ESC[2K` per line is
//     byte-perfect in content and wrong in every cell that relied on that
//     inheritance (see TestNaivePerLineResetVariantIsWrongInColour).
//  4. DECSTBM from the scroll region, AFTER the body -- setting the
//     scroll region homes the cursor, which would misplace anything
//     written after it if it ran before the body instead.
//  5. origin mode, now the pane's real final value (cursor addressing in
//     step 6 depends on it).
//  6. the cursor: position, then visibility.
//  7. the modes that must not disturb the paint above -- none of these
//     change how already-painted cells look, only how FUTURE input is
//     interpreted, so their relative order does not matter, only that
//     they land after every step that does affect the paint.
func BuildSeed(state tmux.PaneSeedState, body []byte) []byte {
	var b bytes.Buffer

	// 1. alternate screen, first.
	writeDECMode(&b, 1049, state.AlternateOn)

	// 2. neutral painting state: DECOM off, full-screen scroll region,
	// DECAWM on, IRM off -- exactly `ESC[?6l ESC[r ESC[?7h ESC[4l`.
	b.WriteString("\x1b[?6l")
	b.WriteString("\x1b[r")
	b.WriteString("\x1b[?7h")
	b.WriteString("\x1b[4l")

	// 3. the capture body, byte for byte -- with one necessary correction:
	// capture-pane's own textual dump joins rows with a bare LF (confirmed
	// directly against real tmux: `od -c` on a captured multi-row body
	// shows plain 0x0A between rows, never 0x0D 0x0A), because that LF is
	// tmux's row SEPARATOR in its snapshot format, not a literal IND
	// control code carried over from whatever originally produced the
	// row break. A real terminal's line discipline (OPOST/ONLCR) supplies
	// the carriage return implicitly for a live byte stream; a stored
	// snapshot has no such layer under it, and x/vt's own LF handling is
	// strict IND (down only, column unchanged) unless ANSI mode 20 (LNM)
	// is set, which it never is here (PaneSeedState carries no LNM field --
	// tmux does not expose one, and no real program sets it). Replaying
	// the bare-LF body directly is therefore not actually verbatim replay
	// of what the pane showed; it silently glues every row after the
	// first to the wrong column (demonstrated by
	// TestBuildSeedReproducesInheritedSGRAcrossLinesWithZeroDifferingCells
	// failing without this line). Restoring the CR the row separator
	// always implied is not "re-addressing" in the sense PRD #18
	// forbids -- no CUP, no SGR reset, no per-line ESC[2K is ever
	// inserted; the SGR-inheritance property #18 exists to protect is
	// completely unaffected by where the cursor's COLUMN resets to
	// between rows.
	//
	// One more correction rides along with it: capture-pane -N always
	// terminates the body with at least one trailing "\n" beyond the
	// separator between the second-to-last and last row (confirmed
	// directly: a 4-row pane's capture carries FOUR "\n" characters, one
	// more than the three separators four rows need). Translated
	// literally, that trailing newline becomes a real CRLF landing past
	// the body's last row, which -- once that row is already occupied --
	// forces an index-triggered scroll that evicts the top row. It is
	// trimmed for the same reason it is not written at all: it marks the
	// end of the snapshot, not one more row to advance into.
	//
	// EXACTLY ONE trailing newline is trimmed (bytes.TrimSuffix), never
	// the whole trailing newline RUN (bytes.TrimRight), which is what
	// this line used to do (issue #29). A capture's trailing BLANK ROWS
	// are real rows -N deliberately preserves, and each of them is a bare
	// empty line in the body, so TrimRight deleted all of them along with
	// the terminator. That was invisible while the body was the visible
	// screen only -- its content lands on the grid's TOP rows either way
	// -- and fatal the moment history is prepended (the entry seed's
	// range, tmux.SeedCaptureOptionsWithHistory): with more body rows
	// than the grid has, the rows are BOTTOM-anchored, so dropping K
	// trailing blank rows shifts the entire picture DOWN by K and pushes
	// the pane's live screen up into the grid's scrollback -- precisely
	// the "the preview is offset and the bottom is missing" failure the
	// history pull would otherwise have introduced. (The bug was also
	// data-dependent, which is why no existing test caught it: with -e, a
	// blank row carrying a BACKGROUND COLOUR is a non-empty string
	// TrimRight never touches. Only default-styled blank tails vanished.)
	//
	// The invariant this trim now establishes, and which both body
	// producers are held to (tmux.Client.CapturePane and
	// tmux.Client.CapturePaneSeedAtomic return byte-identical bodies,
	// pinned by internal/tmux's
	// TestCapturePaneSeedAtomicMatchesSeparateStateAndBodyReads): after
	// the trim the body is exactly R rows joined by R-1 newlines, so R-1
	// CRLFs are written, the cursor ends up ON the last body row with no
	// extra index past it, and every blank row the pane really had is
	// still there.
	b.Write(bytes.ReplaceAll(bytes.TrimSuffix(body, []byte("\n")), []byte("\n"), []byte("\r\n")))

	// 4. DECSTBM from the scroll region, after the body. tmux's
	// scroll_region_upper/lower are 0-based inclusive row indices; DECSTBM
	// takes 1-based rows.
	fmt.Fprintf(&b, "\x1b[%d;%dr", state.ScrollRegionUpper+1, state.ScrollRegionLower+1)

	// 5. origin mode, the pane's real final value.
	writeDECMode(&b, 6, state.OriginFlag)

	// 6. cursor position, then visibility. tmux's cursor_x/cursor_y are
	// 0-based; CUP takes 1-based row;col.
	fmt.Fprintf(&b, "\x1b[%d;%dH", state.CursorY+1, state.CursorX+1)
	writeDECMode(&b, 25, state.CursorFlag)

	// 7. modes that must not disturb the paint: wrap, insert (ECMA-48
	// IRM -- SM/RM, not a DEC private mode, hence no '?'), the keypad
	// flags, and the five independent mouse-tracking flags.
	writeDECMode(&b, 7, state.WrapFlag)
	writeANSIMode(&b, 4, state.InsertFlag)
	writeDECMode(&b, 1, state.KeypadCursorFlag)
	if state.KeypadFlag {
		b.WriteString("\x1b=") // DECKPAM
	} else {
		b.WriteString("\x1b>") // DECKPNM
	}
	writeDECMode(&b, 1000, state.MouseStandardFlag)
	writeDECMode(&b, 1002, state.MouseButtonFlag)
	writeDECMode(&b, 1003, state.MouseAnyFlag)
	writeDECMode(&b, 1005, state.MouseUTF8Flag)
	writeDECMode(&b, 1006, state.MouseSGRFlag)

	return b.Bytes()
}

// writeDECMode writes CSI ? <n> h/l -- a DEC private mode set (h) or reset
// (l).
func writeDECMode(b *bytes.Buffer, n int, on bool) {
	if on {
		fmt.Fprintf(b, "\x1b[?%dh", n)
	} else {
		fmt.Fprintf(b, "\x1b[?%dl", n)
	}
}

// writeANSIMode writes CSI <n> h/l -- an ECMA-48 (ANSI, non-DEC-private)
// mode set (SM) or reset (RM).
func writeANSIMode(b *bytes.Buffer, n int, on bool) {
	if on {
		fmt.Fprintf(b, "\x1b[%dh", n)
	} else {
		fmt.Fprintf(b, "\x1b[%dl", n)
	}
}

// CaptureSeed reads target's current mode state and its VISIBLE SCREEN's
// capture body from a real tmux client and assembles them into a seed via
// BuildSeed. State and body are paired atomically (PRD II-20):
// CapturePaneSeedAtomic chains both tmux calls into one invocation and
// retries while its before/after #{history_size}/#{pane_width}/
// #{pane_height}/#{alternate_on} probes disagree, so the state and the
// body handed to BuildSeed are never a state read against a body the pane
// had already moved past.
//
// Visible-screen-only is what the two PERIODIC reseed loops want
// (captureLoop under TransportCapture, and fallbackLoop after the pipe is
// displaced): both of them rebuild the grid wholesale every 200ms, and
// tmux.SeedCaptureOptions' own doc carries the measured per-tick cost
// that keeps them on this range. A grid seeded this way therefore has an
// EMPTY scrollback and always will (a body exactly as tall as the grid
// never scrolls a row off it) -- pinned, deliberately, by
// TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty. The ENTRY seed uses
// CaptureSeedWithHistory instead; that is issue #29's whole fix.
func CaptureSeed(ctx context.Context, client tmux.Client, target string) ([]byte, error) {
	state, body, err := client.CapturePaneSeedAtomic(ctx, target, tmux.SeedCaptureOptions())
	if err != nil {
		return nil, fmt.Errorf("capture seed for %q: %w", target, err)
	}
	return BuildSeed(state, body), nil
}

// CaptureSeedWithHistory is CaptureSeed over a range that also includes up
// to historyLines rows of the pane's tmux-side scrollback
// (tmux.SeedCaptureOptionsWithHistory), which is what issue #29's
// one-off ENTRY seed needs: everything the pane printed before the
// preview was ever opened lives in tmux's history and nowhere in deck's
// grid, so without this the freshly entered interactive preview's own
// scrollback starts empty and Shift+PgUp/wheel-up reaches nothing at all.
// The extra rows are bottom-anchored by construction -- the body's last
// row is still the pane's last row, so the live screen lands exactly
// where a visible-only seed would have put it and the history goes into
// the grid's scrollback above it (BuildSeed's step 3 documents the trim
// rule that makes that true).
//
// GRACEFUL DEGRADATION, and the reason this is not just a one-line
// options swap: a wider capture takes longer, so it widens
// CapturePaneSeedAtomic's own retry window, and a chatty pane can
// genuinely exhaust maxPaneSeedAtomicAttempts (#{history_size} is a
// discriminator, and it does not even move monotonically -- the shell's
// `clear` emits ESC[3J, which makes tmux ZERO the pane's history). The
// caller of the entry seed turns any error into a refusal to enter
// interactive mode at all ("Cannot enter interactive mode: ...", see
// internal/tui's enterInteractiveBody), and refusing entry to a busy pane
// would be a far worse regression than entering it without history. So
// THAT failure -- and only that one -- is retried once at
// historyLines = 0, the exact range and cost CaptureSeed has always used,
// and that seed is returned instead.
//
// "Only that one" is load-bearing, not tidiness. The degradation used to
// fire on ANY error, which turned every unrelated failure into two
// sequential timeouts: measured against a tmux that never answers,
// CaptureSeed took 5.00s (tmux.Client's own default timeout) while
// CaptureSeedWithHistory took 10.01s, exactly twice. Production runs with
// that 5s default and enterInteractiveBody hands the seed closure a
// deadline-less context.Background(), so pressing Enter at an
// unresponsive tmux server froze bubbletea's whole blocking Update
// goroutine for ten seconds before the error message could render. A dead
// transport, a hung server, an invalid pane id and a cancelled context
// are all things a second attempt cannot fix, so they are returned
// immediately; tmux.PaneSeedProbesNeverAgreedError (errors.As, so the
// classification survives any wrapping tmux adds later) is the one
// "the pane kept moving underneath us" signal that a NARROWER capture
// really can fix, because a narrower capture is a shorter race window.
// That premise is checked rather than assumed: the error carries the
// range it actually failed on, and a failure that was already at the
// visible-only range has nothing narrower to degrade to (see the
// alternate-screen case in the body below).
//
// The fallback's OWN failure is never swallowed: if the visible-only
// retry fails too, the pane is genuinely unreadable (gone, or the socket
// is broken) and that error is returned, naming both attempts so the
// history-inclusive failure is not lost from the message.
func CaptureSeedWithHistory(ctx context.Context, client tmux.Client, target string, historyLines int) ([]byte, error) {
	state, body, err := client.CapturePaneSeedAtomic(ctx, target, tmux.SeedCaptureOptionsWithHistory(historyLines))
	if err == nil {
		return BuildSeed(state, body), nil
	}
	if historyLines <= 0 {
		// Nothing to degrade to: this WAS the visible-only capture (see
		// SeedCaptureOptionsWithHistory, which returns exactly
		// SeedCaptureOptions() here), so a second identical attempt would
		// only double the latency of a failure the caller is about to
		// report anyway.
		return nil, fmt.Errorf("capture seed for %q: %w", target, err)
	}
	var neverAgreed *tmux.PaneSeedProbesNeverAgreedError
	if !errors.As(err, &neverAgreed) {
		// Not a busy pane: a narrower range would fail exactly the same
		// way and cost the caller a second full timeout to find out.
		return nil, fmt.Errorf("capture seed for %q: %w", target, err)
	}
	if neverAgreed.Options == tmux.SeedCaptureOptions() {
		// The pairing that ran out of attempts was ALREADY the
		// visible-only one, so there is nothing narrower left to try. This
		// happens on an alternate-screen pane: CapturePaneSeedAtomic
		// answers a history-inclusive request for such a pane by
		// re-capturing visible-only itself (tmux returns stale
		// pre-launch history above the alternate screen, and a Go-side
		// slice of an `-e` capture would break its cross-row SGR
		// inheritance), and it is that inner capture which can exhaust
		// maxPaneSeedAtomicAttempts. Re-running it from here would spend
		// another twenty attempts on a byte-identical request and then
		// report "history-inclusive capture failed ... and the
		// visible-only retry failed too", naming two ranges when only one
		// was ever tried.
		return nil, fmt.Errorf("capture seed for %q: %w", target, err)
	}
	state, body, fallbackErr := client.CapturePaneSeedAtomic(ctx, target, tmux.SeedCaptureOptions())
	if fallbackErr != nil {
		return nil, fmt.Errorf("capture seed for %q: history-inclusive capture failed (%v) and the visible-only retry failed too: %w", target, err, fallbackErr)
	}
	return BuildSeed(state, body), nil
}

// EntrySeedHistoryLines is how many rows of tmux-side scrollback the
// ONE-OFF entry seed should ask CaptureSeedWithHistory for, given the
// transport the Session about to be started will run under. It exists as
// a function in this package, rather than as a conditional at
// internal/tui's single call site, because the number is a cost decision
// this package owns and measured, and because a decision expressed in
// another package's closure cannot be pinned by a test that can see
// captureLoop.
//
// TransportPipe gets ScrollbackMaxLines -- exactly what the grid can
// hold, so nothing is pulled that would be discarded on arrival -- and
// that is issue #29's whole fix: under the pipe transport the entry seed
// is the ONLY thing that ever writes the pane's pre-entry output into the
// grid, since pipe-pane streams only what the pane prints from then on.
//
// TransportCapture gets 0, and that is not a partial revert of issue #29
// but a refusal to pay for something the transport itself throws away.
// captureLoop replaces the grid WHOLESALE from a visible-only capture on
// its first 200ms tick, so at the operator's geometry the history the
// entry seed pulled (measured 15.3ms of capture plus 94.0ms of grid
// writing, ~18MiB retained, all of it on bubbletea's blocking Update
// goroutine) survived for one fifth of a second and then went away:
// ScrollbackLen 463 -> 0. Nothing an operator could ever scroll to. The
// interactive scrollback is empty under TransportCapture either way --
// that is the deliberate, cost-driven half of issue #29's design
// (SeedCaptureOptions' own doc carries the per-tick measurement, and
// TestVisibleOnlyCaptureSeedLeavesScrollbackEmpty pins it) -- so the only
// thing the history pull bought here was the hitch on Enter.
//
// tmux.SeedCaptureOptionsWithHistory(0) degenerates to exactly
// SeedCaptureOptions(), so 0 means the identical range, and identical
// cost, that the pre-issue-#29 entry seed used.
func EntrySeedHistoryLines(transport Transport) int {
	if transport == TransportCapture {
		return 0
	}
	return ScrollbackMaxLines
}
