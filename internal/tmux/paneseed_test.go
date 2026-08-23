// paneseed_test.go proves PRD phase3b II-19 directly: PaneSeedState reads
// every field the requirement names -- including the three the shipping
// prior art omits (WrapFlag, OriginFlag, ScrollRegionUpper/Lower) -- and a
// plain capture-pane snapshot carries none of this mode state, which is
// the whole reason a separate read is needed at all.
package tmux

import (
	"context"
	"regexp"
	"testing"
	"time"
)

// runInPaneBlocking types a shell command that first emits raw bytes via
// printf (so they land in the pane's OUTPUT stream and are processed by
// tmux's own terminal emulation exactly like any program's output would
// be, unlike bytes sent as literal keyboard INPUT) and then blocks on
// `cat > /dev/null` forever, so the shell never redraws its next prompt
// and perturbs cursor position or any mode this test just set. The pane
// is left with that blocking command running; the caller's session
// cleanup (kill-server) is what ends it.
func runInPaneBlocking(t *testing.T, socket, target, rawBytesShellLiteral string) {
	t.Helper()
	cmd := "printf \"" + rawBytesShellLiteral + "\"; cat > /dev/null"
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", cmd)
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
	time.Sleep(300 * time.Millisecond)
}

// TestPaneSeedStateReadsDefaultState pins the baseline every non-default
// case below diffs against, so a later "reads a non-default value" claim
// is provably a change from something, not a coincidence.
func TestPaneSeedStateReadsDefaultState(t *testing.T) {
	socket := geometrySocket("seed-default")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	time.Sleep(200 * time.Millisecond)
	state, err := client.PaneSeedState(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}

	if state.AlternateOn {
		t.Errorf("AlternateOn = true, want false by default")
	}
	if !state.CursorFlag {
		t.Errorf("CursorFlag = false, want true (cursor visible) by default")
	}
	if state.InsertFlag {
		t.Errorf("InsertFlag = true, want false by default")
	}
	if state.KeypadCursorFlag {
		t.Errorf("KeypadCursorFlag = true, want false by default")
	}
	if state.KeypadFlag {
		t.Errorf("KeypadFlag = true, want false by default")
	}
	if state.MouseAnyFlag || state.MouseButtonFlag || state.MouseSGRFlag || state.MouseStandardFlag || state.MouseUTF8Flag {
		t.Errorf("a mouse flag is true, want every mouse flag false by default: %+v", state)
	}
	if !state.WrapFlag {
		t.Errorf("WrapFlag = false, want true (DECAWM on) by default")
	}
	if state.OriginFlag {
		t.Errorf("OriginFlag = true, want false by default")
	}
	if state.ScrollRegionUpper != 0 {
		t.Errorf("ScrollRegionUpper = %d, want 0 by default", state.ScrollRegionUpper)
	}
	if state.ScrollRegionLower != 9 {
		t.Errorf("ScrollRegionLower = %d, want 9 (height 10, 0-based, inclusive) by default", state.ScrollRegionLower)
	}
}

// TestPaneSeedStateReadsEachNonDefaultFieldFromARealPane is the
// non-vacuous proof PRD II-19 demands for every field: a real escape
// sequence is emitted from inside a real pane, and PaneSeedState is
// asserted to read back the resulting non-default value for exactly the
// field(s) that sequence affects.
func TestPaneSeedStateReadsEachNonDefaultFieldFromARealPane(t *testing.T) {
	cases := []struct {
		name    string
		bytes   string // shell double-quote literal passed to printf
		check   func(PaneSeedState) bool
		explain string
	}{
		{
			name:    "alternate_on",
			bytes:   `\033[?1049h`,
			check:   func(s PaneSeedState) bool { return s.AlternateOn },
			explain: "AlternateOn",
		},
		{
			name:  "cursor_x_and_cursor_y",
			bytes: `\033[5;10H`,
			check: func(s PaneSeedState) bool {
				return s.CursorX == 9 && s.CursorY == 4
			},
			explain: "CursorX==9 && CursorY==4",
		},
		{
			name:    "cursor_flag",
			bytes:   `\033[?25l`,
			check:   func(s PaneSeedState) bool { return !s.CursorFlag },
			explain: "CursorFlag false (cursor hidden)",
		},
		{
			name:    "insert_flag",
			bytes:   `\033[4h`,
			check:   func(s PaneSeedState) bool { return s.InsertFlag },
			explain: "InsertFlag",
		},
		{
			name:    "keypad_cursor_flag",
			bytes:   `\033[?1h`,
			check:   func(s PaneSeedState) bool { return s.KeypadCursorFlag },
			explain: "KeypadCursorFlag",
		},
		{
			name:    "keypad_flag",
			bytes:   `\033=`,
			check:   func(s PaneSeedState) bool { return s.KeypadFlag },
			explain: "KeypadFlag",
		},
		{
			name:    "mouse_any_flag",
			bytes:   `\033[?1003h`,
			check:   func(s PaneSeedState) bool { return s.MouseAnyFlag },
			explain: "MouseAnyFlag",
		},
		{
			name:    "mouse_button_flag",
			bytes:   `\033[?1002h`,
			check:   func(s PaneSeedState) bool { return s.MouseButtonFlag },
			explain: "MouseButtonFlag",
		},
		{
			name:    "mouse_sgr_flag",
			bytes:   `\033[?1006h`,
			check:   func(s PaneSeedState) bool { return s.MouseSGRFlag },
			explain: "MouseSGRFlag",
		},
		{
			name:    "mouse_standard_flag",
			bytes:   `\033[?1000h`,
			check:   func(s PaneSeedState) bool { return s.MouseStandardFlag },
			explain: "MouseStandardFlag",
		},
		{
			name:    "mouse_utf8_flag",
			bytes:   `\033[?1005h`,
			check:   func(s PaneSeedState) bool { return s.MouseUTF8Flag },
			explain: "MouseUTF8Flag",
		},
		{
			name:    "wrap_flag",
			bytes:   `\033[?7l`,
			check:   func(s PaneSeedState) bool { return !s.WrapFlag },
			explain: "WrapFlag false (DECAWM off)",
		},
		{
			name:    "origin_flag",
			bytes:   `\033[?6h`,
			check:   func(s PaneSeedState) bool { return s.OriginFlag },
			explain: "OriginFlag",
		},
		{
			name:  "scroll_region_upper_and_lower",
			bytes: `\033[3;7r`,
			check: func(s PaneSeedState) bool {
				return s.ScrollRegionUpper == 2 && s.ScrollRegionLower == 6
			},
			explain: "ScrollRegionUpper==2 && ScrollRegionLower==6",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := geometrySocket("seed-" + tc.name)
			cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
			defer cleanup()

			client := Client{Socket: socket, Timeout: 5 * time.Second}
			ctx := context.Background()

			runInPaneBlocking(t, socket, "s0", tc.bytes)

			state, err := client.PaneSeedState(ctx, "s0")
			if err != nil {
				t.Fatalf("PaneSeedState: %v", err)
			}
			if !tc.check(state) {
				t.Fatalf("after emitting %s, want %s, got %+v", tc.bytes, tc.explain, state)
			}
		})
	}
}

// Dangerous escape classes a plain `capture-pane -p -e` snapshot must
// NEVER contain, per PRD II-19's claim that a capture carries cell
// content and SGR only: a DEC private mode set/reset (CSI ? ... h/l), a
// DECSTBM scroll-region program (CSI ... r) and absolute cursor
// positioning (CSI ... H or CSI ... f). SGR itself (CSI ... m) is exactly
// what a capture SHOULD carry and is deliberately not in this list.
var (
	decPrivateModePattern = regexp.MustCompile(`\x1b\[\?[0-9;]+[hl]`)
	decstbmPattern        = regexp.MustCompile(`\x1b\[[0-9;]*r`)
	cursorPositionPattern = regexp.MustCompile(`\x1b\[[0-9;]*[Hf]`)
)

// TestCapturePaneCarriesNoModeState is the negative half of PRD II-19's
// claim: it is the reason PaneSeedState needs to exist at all, rather
// than deriving every field from the capture body itself. A pane is
// driven into a pile of non-default mode state (alternate screen, a
// custom scroll region, origin mode, hidden cursor, insert mode, mouse
// tracking) and then captured; the captured bytes must show zero
// occurrences of every dangerous class above, proving none of that state
// is recoverable from the capture.
func TestCapturePaneCarriesNoModeState(t *testing.T) {
	socket := geometrySocket("seed-capture-carries-nothing")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()

	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Colour text (SGR) deliberately included alongside the dangerous
	// modes: the point is that SGR DOES survive (it belongs in a
	// capture) while none of the mode-setting sequences do.
	runInPaneBlocking(t, socket, "s0",
		`\033[?1049h\033[3;7r\033[?6h\033[4h\033[?1h\033[?1002h\033[?7l\033[?25l\033[31mRED\033[0m`)

	state, err := client.PaneSeedState(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneSeedState: %v", err)
	}
	if !state.AlternateOn || state.ScrollRegionUpper != 2 || state.ScrollRegionLower != 6 || !state.OriginFlag {
		t.Fatalf("test setup did not reach the intended mode state, control is not meaningful: %+v", state)
	}

	captured, err := client.CapturePane(ctx, "%0", CaptureOptions{StartLine: "-", EndLine: "-", IncludeEscapeSequences: true})
	if err != nil {
		// %0 is this session's only pane; fall back to resolving it by
		// name if the hardcoded pane id ever stops matching.
		sessions, listErr := client.List(ctx)
		if listErr != nil || len(sessions) == 0 || len(sessions[0].Panes) == 0 {
			t.Fatalf("CapturePane(%%0): %v (and List fallback failed: %v)", err, listErr)
		}
		captured, err = client.CapturePane(ctx, sessions[0].Panes[0].ID, CaptureOptions{StartLine: "-", EndLine: "-", IncludeEscapeSequences: true})
		if err != nil {
			t.Fatalf("CapturePane: %v", err)
		}
	}

	if !regexp.MustCompile(`RED`).Match(captured) {
		t.Fatalf("captured pane does not even contain the literal text written to it; test assumption violated:\n%q", captured)
	}
	if m := decPrivateModePattern.FindAll(captured, -1); len(m) > 0 {
		t.Errorf("capture contains %d DEC private mode sequence(s), want zero: %q", len(m), m)
	}
	if m := decstbmPattern.FindAll(captured, -1); len(m) > 0 {
		t.Errorf("capture contains %d DECSTBM scroll-region sequence(s), want zero: %q", len(m), m)
	}
	if m := cursorPositionPattern.FindAll(captured, -1); len(m) > 0 {
		t.Errorf("capture contains %d cursor-positioning sequence(s), want zero: %q", len(m), m)
	}
}

// TestParsePaneSeedStateRejectsWrongFieldCount guards the wire format
// itself: if a future edit adds or removes a `#{...}` term from
// paneSeedStateFormat without updating parsePaneSeedState (or vice
// versa), this fails loudly instead of silently misassigning fields.
func TestParsePaneSeedStateRejectsWrongFieldCount(t *testing.T) {
	if _, err := parsePaneSeedState("0|0|0"); err == nil {
		t.Fatalf("parsePaneSeedState with too few fields: want error, got nil")
	}
}
