package interactive

import (
	"bytes"
	"context"
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
	// the pane's last row, which -- once the pane's own last row is
	// already occupied -- forces an index-triggered scroll that evicts
	// row zero. It is trimmed first for the same reason it is not
	// written at all: it marks the end of the snapshot, not one more row
	// to advance into.
	b.Write(bytes.ReplaceAll(bytes.TrimRight(body, "\n"), []byte("\n"), []byte("\r\n")))

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

// CaptureSeed reads target's current mode state and capture body from a
// real tmux client and assembles them into a seed via BuildSeed. The two
// reads are two separate tmux invocations here -- task 043/II-20 is what
// pairs them atomically and retries on disagreement; this is the
// unpaired building block that task depends on.
func CaptureSeed(ctx context.Context, client tmux.Client, target string) ([]byte, error) {
	state, err := client.PaneSeedState(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("capture seed state for %q: %w", target, err)
	}
	body, err := client.CapturePane(ctx, target, tmux.SeedCaptureOptions())
	if err != nil {
		return nil, fmt.Errorf("capture seed body for %q: %w", target, err)
	}
	return BuildSeed(state, body), nil
}
