// seed_test.go proves PRD phase3b II-17/II-18 directly: BuildSeed emits
// the specified escape sequence in the specified order, the capture body
// goes in verbatim (never re-addressed or SGR-reset per line), and both
// of the PRD's own mandatory negative controls (the naive per-line-reset
// variant, and dropping `-N`) are demonstrated genuinely wrong.
package interactive

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// firstPaneID resolves a bare session's own pane_id, which
// Client.CapturePane requires (unlike PaneSeedState/display-message, it
// only accepts a real "%N" target, not a session name).
func firstPaneID(t *testing.T, socket, session string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "list-panes", "-t", session, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes -t %s: %v", session, err)
	}
	id := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if id == "" {
		t.Fatalf("list-panes -t %s returned no pane_id", session)
	}
	return id
}

// runShellPrintf types a shell `printf` command containing rawEscapes
// (already backslash-escaped for the shell, e.g. `\\033[44m`) into the
// pane as literal keystrokes, so tmux's own terminal emulation processes
// the resulting bytes as PANE OUTPUT exactly like any real program's
// output would (the same technique internal/tmux/paneseed_test.go's
// runInPaneBlocking uses).
func runShellPrintf(t *testing.T, socket, target, rawEscapes string) {
	t.Helper()
	cmd := fmt.Sprintf(`printf "%s"`, rawEscapes)
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "-l", "--", cmd).CombinedOutput(); err != nil {
		t.Fatalf("send-keys -l -- %q: %v: %s", cmd, err, out)
	}
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("send-keys Enter: %v: %s", err, out)
	}
	time.Sleep(300 * time.Millisecond)
}

// cellBg returns cell(x,y)'s background colour as an RGBA tuple, or ok
// false if the cell has no background set at all (the "default"/no-op
// case, as opposed to an explicit colour that happens to render the same
// as the terminal default).
func cellBg(g *Grid, x, y int) (r, gg, b, a uint32, ok bool) {
	cell := g.CellAt(x, y)
	if cell == nil || cell.Style.Bg == nil {
		return 0, 0, 0, 0, false
	}
	r, gg, b, a = cell.Style.Bg.RGBA()
	return r, gg, b, a, true
}

func cellText(g *Grid, x, y int) string {
	cell := g.CellAt(x, y)
	if cell == nil {
		return " "
	}
	if cell.Content == "" {
		return " "
	}
	return cell.Content
}

// diffCells reports the (x, y) coordinates where a's and b's cells differ
// in content or background colour, over the given width/height -- the
// "zero differing cells" comparison the task criteria names, done cell by
// cell rather than by comparing rendered strings.
func diffCells(a, b *Grid, width, height int) [][2]int {
	var diffs [][2]int
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if cellText(a, x, y) != cellText(b, x, y) {
				diffs = append(diffs, [2]int{x, y})
				continue
			}
			ar, ag, ab, aa, aok := cellBg(a, x, y)
			br, bg, bb, ba, bok := cellBg(b, x, y)
			if aok != bok || ar != br || ag != bg || ab != bb || aa != ba {
				diffs = append(diffs, [2]int{x, y})
			}
		}
	}
	return diffs
}

// TestBuildSeedReproducesInheritedSGRAcrossLinesWithZeroDifferingCells is
// II-17/II-18's primary positive case. A real bare pane is driven into a
// state real tmux only produces when a coloured row exactly fills the
// pane's width: `capture-pane -e`'s SECOND row carries no SGR of its own
// at all (verified directly against real tmux below), relying entirely on
// inheriting the pen from row one's last cell -- exactly the shipping
// prior art defect PRD #18 names. BuildSeed's verbatim body (never
// re-addressed, never reset per line) must reproduce that inheritance
// with zero differing cells against a grid fed the SAME raw body with no
// wrapping at all (the wrapping steps are mode/cursor bookkeeping around
// an already-neutral fresh grid; they must never touch the painted
// cells).
func TestBuildSeedReproducesInheritedSGRAcrossLinesWithZeroDifferingCells(t *testing.T) {
	socket := interactiveSocket("seed-verbatim")
	width, height := 10, 4
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	// Row 1: full-width blue background, "AAAAAAAAAA". Row 2:
	// "BBBBBBBBBB" with NO colour code of its own -- but because row 1's
	// blue ran to the pane's exact width, tmux's `-e` capture will still
	// render row 2 blue by inheritance (confirmed below).
	runShellPrintf(t, socket, "s0", `\033[2J\033[H\033[44mAAAAAAAAAA\033[44mBBBBBBBBBB\033[0m\n`)

	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	body, err := client.CapturePane(ctx, paneID, tmux.SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePane (seed options): %v", err)
	}
	if !bytes.Contains(body, []byte("AAAAAAAAAA")) || !bytes.Contains(body, []byte("BBBBBBBBBB")) {
		t.Fatalf("seed capture body = %q, want both rows' text", body)
	}
	// Confirm the inheritance premise directly (row 2's raw capture text
	// must not carry its own "\x1b[44m" -- if it did, this test would
	// prove nothing about inheritance, only about literal SGR replay).
	lines := bytes.Split(body, []byte("\n"))
	if len(lines) < 2 || bytes.Contains(lines[1], []byte("\x1b[44m")) {
		t.Fatalf("test assumption violated: real capture's row 2 = %q, want it to inherit blue with no SGR of its own", lines[1])
	}

	state, err := client.PaneSeedState(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}

	seed := BuildSeed(state, body)

	seeded := newGrid(width, height)
	if _, err := seeded.Write(seed); err != nil {
		t.Fatalf("write seed into grid: %v", err)
	}

	// The comparison target: a second fresh grid fed the body with the
	// same CR-restoration BuildSeed itself applies (bare "\n" is a row
	// separator in capture-pane's textual format, not a literal terminal
	// control code -- see BuildSeed's own comment), but NONE of BuildSeed's
	// other wrapping escapes at all. A fresh grid already starts in the
	// "neutral painting state" BuildSeed's step 2 sets explicitly, so if
	// the wrapping steps are doing their job (painting-inert bookkeeping
	// only) the two must be cell-for-cell identical.
	plain := newGrid(width, height)
	if _, err := plain.Write(bytes.ReplaceAll(bytes.TrimRight(body, "\n"), []byte("\n"), []byte("\r\n"))); err != nil {
		t.Fatalf("write plain body into grid: %v", err)
	}

	if diffs := diffCells(seeded, plain, width, height); len(diffs) != 0 {
		t.Fatalf("seeded grid differs from plain-body grid at %d cells (want 0): %v", len(diffs), diffs)
	}

	// And the inheritance itself actually reached the grid: row 2, every
	// column, must carry the same blue background as row 1.
	wantR, wantG, wantB, wantA, ok := cellBg(seeded, 0, 0)
	if !ok {
		t.Fatalf("row 1 col 0 has no background at all; test fixture did not produce the colour it depends on")
	}
	for x := 0; x < width; x++ {
		gotR, gotG, gotB, gotA, ok := cellBg(seeded, x, 1)
		if !ok || gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
			t.Errorf("seeded grid row 2 col %d background = (%v,%v,%v,%v ok=%v), want inherited blue (%v,%v,%v,%v)", x, gotR, gotG, gotB, gotA, ok, wantR, wantG, wantB, wantA)
		}
	}
}

// TestNaivePerLineResetVariantIsWrongInColour is PRD #18's mandatory red
// control: rebuilding the seed by re-addressing and SGR-resetting each
// line of the SAME real capture is byte-perfect in content (every
// character is exactly where BuildSeed's verbatim version puts it) and
// wrong in colour (row 2 loses the inherited blue it must keep), proving
// the "verbatim, never re-addressed" rule is load-bearing rather than
// stylistic.
func TestNaivePerLineResetVariantIsWrongInColour(t *testing.T) {
	socket := interactiveSocket("seed-naive-control")
	width, height := 10, 4
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	runShellPrintf(t, socket, "s0", `\033[2J\033[H\033[44mAAAAAAAAAA\033[44mBBBBBBBBBB\033[0m\n`)

	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	body, err := client.CapturePane(ctx, paneID, tmux.SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePane (seed options): %v", err)
	}
	state, err := client.PaneSeedState(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}

	correct := newGrid(width, height)
	if _, err := correct.Write(BuildSeed(state, body)); err != nil {
		t.Fatalf("write correct seed into grid: %v", err)
	}

	// The naive per-line variant PRD #18 names: split the SAME verbatim
	// body by line, and re-address + SGR-reset + clear-to-end-of-line
	// before each one, exactly as `ESC[<row>;1H ESC[0m ESC[2K` +
	// line-content would.
	naiveLines := bytes.Split(bytes.TrimRight(body, "\n"), []byte("\n"))
	var naive bytes.Buffer
	for i, line := range naiveLines {
		fmt.Fprintf(&naive, "\x1b[%d;1H\x1b[0m\x1b[2K", i+1)
		naive.Write(line)
		naive.WriteString("\r\n")
	}

	naiveGrid := newGrid(width, height)
	if _, err := naiveGrid.Write(naive.Bytes()); err != nil {
		t.Fatalf("write naive per-line variant into grid: %v", err)
	}

	// Content: byte-perfect. Every cell's TEXT must match.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if cellText(correct, x, y) != cellText(naiveGrid, x, y) {
				t.Fatalf("naive variant's content diverged at (%d,%d): %q vs %q -- the control is supposed to be byte-perfect in CONTENT, only wrong in colour", x, y, cellText(naiveGrid, x, y), cellText(correct, x, y))
			}
		}
	}

	// Colour: wrong. Row 2 (index 1) must have LOST the inherited blue in
	// the naive variant, while the correct seed still has it.
	_, _, _, _, correctHasBg := cellBg(correct, 0, 1)
	_, _, _, _, naiveHasBg := cellBg(naiveGrid, 0, 1)
	if !correctHasBg {
		t.Fatalf("test assumption violated: the correctly-seeded grid itself lost row 2's inherited background; nothing to contrast against")
	}
	if naiveHasBg {
		t.Fatalf("naive per-line-reset variant row 2 still carries a background colour; want it lost (ESC[0m resets the pen before a line that never restates its own SGR)")
	}
}

// TestDroppingPreserveTrailingBlankLinesTrimsBackgroundStyledBlanks is PRD
// #18's second mandatory red control: without `-N`, tmux trims trailing
// background-styled blank cells from a capture, so a seed built from that
// capture leaves those cells at the grid's own default instead of the
// pane's real colour.
func TestDroppingPreserveTrailingBlankLinesTrimsBackgroundStyledBlanks(t *testing.T) {
	socket := interactiveSocket("seed-dash-n-control")
	width, height := 10, 4
	cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
	defer cleanup()

	// A short glyph followed by nothing else on the line, but with the
	// background colour set BEFORE it and never reset -- so every column
	// after the glyph is a background-styled blank, trimmed by tmux
	// unless -N is given.
	runShellPrintf(t, socket, "s0", `\033[44m\033[2J\033[HAB\n`)

	paneID := firstPaneID(t, socket, "s0")
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	withN, err := client.CapturePane(ctx, paneID, tmux.SeedCaptureOptions())
	if err != nil {
		t.Fatalf("CapturePane (-N): %v", err)
	}
	withoutN, err := client.CapturePane(ctx, paneID, tmux.CaptureOptions{StartLine: "0", EndLine: "-", IncludeEscapeSequences: true})
	if err != nil {
		t.Fatalf("CapturePane (no -N): %v", err)
	}
	if bytes.Equal(withN, withoutN) {
		t.Fatalf("test assumption violated: -N made no difference to the raw capture, so this control proves nothing")
	}

	state, err := client.PaneSeedState(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}

	preserved := newGrid(width, height)
	if _, err := preserved.Write(BuildSeed(state, withN)); err != nil {
		t.Fatalf("write -N seed into grid: %v", err)
	}
	trimmed := newGrid(width, height)
	if _, err := trimmed.Write(BuildSeed(state, withoutN)); err != nil {
		t.Fatalf("write non--N seed into grid: %v", err)
	}

	// Column 9 (last column, past "AB") is a background-styled blank on
	// the real pane. -N must preserve it; dropping -N must lose it.
	if _, _, _, _, ok := cellBg(preserved, width-1, 0); !ok {
		t.Fatalf("with -N: trailing background-styled blank at column %d was NOT preserved; the control's premise is broken", width-1)
	}
	if _, _, _, _, ok := cellBg(trimmed, width-1, 0); ok {
		t.Fatalf("without -N: trailing background-styled blank at column %d survived anyway; want it trimmed (that's the whole point of this control)", width-1)
	}
}

// TestBuildSeedFieldOrderAndPerFieldEscapes is a pure, deterministic
// structural check of BuildSeed's own output: every one of
// tmux.PaneSeedState's sixteen fields produces the exact escape PRD #17
// specifies, in the exact relative order PRD #17 specifies, for both the
// "true" and "false" case of every boolean field. This is the level at
// which mode-state correctness for fields the x/vt emulator does not
// expose a public getter for (grep of safe_emulator.go/emulator.go shows
// no exported cursor-visibility, insert-mode, wrap-mode, keypad or mouse
// getter) can be verified directly, rather than only observed indirectly;
// WrapFlag/OriginFlag are additionally proven BEHAVIOURALLY against the
// real emulator below (TestBuildSeedWrapFlag..., ...Origin...) because
// those two ARE observable that way; InsertFlag is not (see the comment
// above TestBuildSeedWrapFlagControlsSubsequentLineWrap for why), so this
// structural check is the only proof it gets.
func TestBuildSeedFieldOrderAndPerFieldEscapes(t *testing.T) {
	body := []byte("BODY")
	allOff := tmux.PaneSeedState{ScrollRegionUpper: 0, ScrollRegionLower: 23}
	allOn := tmux.PaneSeedState{
		AlternateOn: true, CursorX: 5, CursorY: 3, CursorFlag: true, InsertFlag: true,
		KeypadCursorFlag: true, KeypadFlag: true,
		MouseAnyFlag: true, MouseButtonFlag: true, MouseSGRFlag: true, MouseStandardFlag: true, MouseUTF8Flag: true,
		WrapFlag: true, OriginFlag: true, ScrollRegionUpper: 2, ScrollRegionLower: 20,
	}

	for _, tc := range []struct {
		name  string
		state tmux.PaneSeedState
	}{
		{"allOff", allOff},
		{"allOn", allOn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed := string(BuildSeed(tc.state, body))

			mustIndex := func(sub string) int {
				i := strings.Index(seed, sub)
				if i < 0 {
					t.Fatalf("seed does not contain %q: %q", sub, seed)
				}
				return i
			}

			alt := "\x1b[?1049l"
			if tc.state.AlternateOn {
				alt = "\x1b[?1049h"
			}
			iAlt := mustIndex(alt)
			iNeutral6l := mustIndex("\x1b[?6l")
			iNeutralR := mustIndex("\x1b[r")
			iNeutral7h := mustIndex("\x1b[?7h")
			iNeutral4l := mustIndex("\x1b[4l")
			iBody := mustIndex(string(body))

			// Step 1 (alt screen) must precede step 2 (neutral state),
			// which must precede step 3 (the body).
			if !(iAlt < iNeutral6l && iNeutral6l < iNeutralR && iNeutralR < iNeutral7h && iNeutral7h < iNeutral4l && iNeutral4l < iBody) {
				t.Fatalf("seed order violated before the body: alt=%d neutral(?6l=%d r=%d ?7h=%d 4l=%d) body=%d, want strictly increasing", iAlt, iNeutral6l, iNeutralR, iNeutral7h, iNeutral4l, iBody)
			}

			stbm := fmt.Sprintf("\x1b[%d;%dr", tc.state.ScrollRegionUpper+1, tc.state.ScrollRegionLower+1)
			iSTBM := mustIndex(stbm)
			if iSTBM < iBody+len(body) {
				t.Fatalf("DECSTBM at %d overlaps or precedes the end of the body (ends at %d); want it strictly after", iSTBM, iBody+len(body))
			}

			origin := "\x1b[?6l"
			if tc.state.OriginFlag {
				origin = "\x1b[?6h"
			}
			// Search for origin AFTER the STBM sequence, since "\x1b[?6l"
			// also appears earlier as the neutral-state step.
			iOriginRel := strings.Index(seed[iSTBM+len(stbm):], origin)
			if iOriginRel < 0 {
				t.Fatalf("no origin-mode escape %q found after DECSTBM in %q", origin, seed)
			}
			iOrigin := iSTBM + len(stbm) + iOriginRel

			cup := fmt.Sprintf("\x1b[%d;%dH", tc.state.CursorY+1, tc.state.CursorX+1)
			iCUPRel := strings.Index(seed[iOrigin:], cup)
			if iCUPRel < 0 {
				t.Fatalf("no cursor-position escape %q found after origin mode in %q", cup, seed)
			}
			iCUP := iOrigin + iCUPRel

			cursorVis := "\x1b[?25l"
			if tc.state.CursorFlag {
				cursorVis = "\x1b[?25h"
			}
			iCursorVisRel := strings.Index(seed[iCUP:], cursorVis)
			if iCursorVisRel < 0 {
				t.Fatalf("no cursor-visibility escape %q found after cursor position in %q", cursorVis, seed)
			}
			iCursorVis := iCUP + iCursorVisRel

			// Step 7: the remaining modes, in ANY order relative to each
			// other, but ALL strictly after cursor visibility.
			after := seed[iCursorVis+len(cursorVis):]
			checkMode := func(field string, on bool, onSeq, offSeq string) {
				want := offSeq
				if on {
					want = onSeq
				}
				if !strings.Contains(after, want) {
					t.Errorf("%s (%v): seed's trailing modes segment does not contain %q: %q", field, on, want, after)
				}
			}
			checkMode("WrapFlag", tc.state.WrapFlag, "\x1b[?7h", "\x1b[?7l")
			checkMode("InsertFlag", tc.state.InsertFlag, "\x1b[4h", "\x1b[4l")
			checkMode("KeypadCursorFlag", tc.state.KeypadCursorFlag, "\x1b[?1h", "\x1b[?1l")
			if tc.state.KeypadFlag {
				if !strings.Contains(after, "\x1b=") {
					t.Errorf("KeypadFlag true: trailing modes segment does not contain DECKPAM \\x1b=: %q", after)
				}
			} else if !strings.Contains(after, "\x1b>") {
				t.Errorf("KeypadFlag false: trailing modes segment does not contain DECKPNM \\x1b>: %q", after)
			}
			checkMode("MouseStandardFlag", tc.state.MouseStandardFlag, "\x1b[?1000h", "\x1b[?1000l")
			checkMode("MouseButtonFlag", tc.state.MouseButtonFlag, "\x1b[?1002h", "\x1b[?1002l")
			checkMode("MouseAnyFlag", tc.state.MouseAnyFlag, "\x1b[?1003h", "\x1b[?1003l")
			checkMode("MouseUTF8Flag", tc.state.MouseUTF8Flag, "\x1b[?1005h", "\x1b[?1005l")
			checkMode("MouseSGRFlag", tc.state.MouseSGRFlag, "\x1b[?1006h", "\x1b[?1006l")
		})
	}
}

// TestBuildSeedWrapFlagControlsSubsequentLineWrap and its sibling below
// prove "correct mode state" behaviourally against the real x/vt emulator
// for the two fields it DOES expose observable behaviour for (wrap,
// origin+scroll-region), by writing further bytes into the seeded grid
// and checking where they land -- not merely that BuildSeed emitted the
// right bytes (that's TestBuildSeedFieldOrderAndPerFieldEscapes above),
// but that the real emulator actually ends up in that mode.
//
// InsertFlag has no such sibling: reading handlers.go/csi_mode.go in this
// x/vt version shows ICH (Insert Character) is only ever invoked as its
// own explicit escape command (CSI n @); ANSI mode 4 (IRM) is stored in
// e.modes when BuildSeed's \x1b[4h/l sets it but no code path consults
// that stored value on an ordinary character write -- this emulator does
// not implement IRM's automatic shift-right behaviour at all, so there is
// no real behaviour to observe here beyond the escape-emission structural
// check InsertFlag already gets in TestBuildSeedFieldOrderAndPerFieldEscapes.
// (Confirmed by writing a would-be behavioural test and watching it fail
// identically for both insert=true and insert=false before removing it.)
func TestBuildSeedWrapFlagControlsSubsequentLineWrap(t *testing.T) {
	width, height := 5, 3
	// Cursor at the last column of row 0 (0-based x=4,y=0) via the
	// seed's own cursor placement, then two more characters are written
	// AFTER the seed. With wrap on, the second lands at (0,1); with wrap
	// off, both land at (4,0) (the last one overwriting the first).
	for _, wrap := range []bool{true, false} {
		state := tmux.PaneSeedState{CursorX: width - 1, CursorY: 0, CursorFlag: true, WrapFlag: wrap, ScrollRegionLower: height - 1}
		g := newGrid(width, height)
		if _, err := g.Write(BuildSeed(state, nil)); err != nil {
			t.Fatalf("wrap=%v: write seed: %v", wrap, err)
		}
		if _, err := g.Write([]byte("XY")); err != nil {
			t.Fatalf("wrap=%v: write XY: %v", wrap, err)
		}
		gotRow1 := cellText(g, 0, 1)
		if wrap {
			if gotRow1 != "Y" {
				t.Errorf("wrap=true: cell(0,1) = %q, want %q (Y should have wrapped to the next line)", gotRow1, "Y")
			}
		} else {
			if gotRow1 == "Y" {
				t.Errorf("wrap=false: cell(0,1) = %q, want anything but the wrapped %q (autowrap must be off)", gotRow1, "Y")
			}
		}
	}
}

func TestBuildSeedOriginFlagControlsCursorAddressingRelativeToScrollRegion(t *testing.T) {
	width, height := 10, 10
	upper, lower := 3, 7 // 0-based, inclusive
	for _, origin := range []bool{true, false} {
		state := tmux.PaneSeedState{CursorFlag: true, OriginFlag: origin, ScrollRegionUpper: upper, ScrollRegionLower: lower}
		g := newGrid(width, height)
		if _, err := g.Write(BuildSeed(state, nil)); err != nil {
			t.Fatalf("origin=%v: write seed: %v", origin, err)
		}
		// Home the cursor (no parameters -- addressed relative to the
		// scroll region when origin mode is on, absolute otherwise), then
		// write a single marker glyph.
		if _, err := g.Write([]byte("\x1b[H*")); err != nil {
			t.Fatalf("origin=%v: write CUP-home + marker: %v", origin, err)
		}
		wantY := 0
		if origin {
			wantY = upper
		}
		if got := cellText(g, 0, wantY); got != "*" {
			t.Errorf("origin=%v: cell(0,%d) = %q, want the marker %q (home should land at %s)", origin, wantY, got, "*", map[bool]string{true: "the top of the scroll region", false: "the absolute top of the pane"}[origin])
		}
	}
}
