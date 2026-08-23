package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PaneSeedState is every piece of a pane's live terminal-mode state that
// tmux exposes through its own formats and that a `capture-pane` snapshot
// does NOT carry (PRD phase3b II-19): a capture is cell content and SGR
// only -- no DEC private mode, no scroll region, no cursor position
// survives it (proven negatively by TestCapturePaneCarriesNoModeState in
// paneseed_test.go). deck can seed an interactive grid strictly better
// than a plain replay of the capture body because tmux hands these over
// for free through display-message, with no extra pane interaction
// required.
//
// WrapFlag, OriginFlag and ScrollRegionUpper/Lower are the three fields
// the shipping prior art this package improves on omits entirely
// (docs/reports/phase3b.md's II-19 section records the finding).
type PaneSeedState struct {
	// AlternateOn is #{alternate_on}: whether the pane is currently
	// showing its alternate screen buffer (DECSET 1049).
	AlternateOn bool
	// CursorX and CursorY are #{cursor_x}/#{cursor_y}: the cursor's
	// current column/row within the pane, both 0-based.
	CursorX, CursorY int
	// CursorFlag is #{cursor_flag}: whether the cursor is currently
	// visible (DECTCEM, default on).
	CursorFlag bool
	// InsertFlag is #{insert_flag}: whether the pane is in insert mode
	// (IRM).
	InsertFlag bool
	// KeypadCursorFlag is #{keypad_cursor_flag}: DECCKM, application
	// cursor-key mode.
	KeypadCursorFlag bool
	// KeypadFlag is #{keypad_flag}: DECKPAM/DECKPNM, application keypad
	// mode.
	KeypadFlag bool
	// The five mouse-tracking flags tmux tracks independently:
	// MouseAnyFlag (#{mouse_any_flag}, mode 1003), MouseButtonFlag
	// (#{mouse_button_flag}, mode 1002), MouseSGRFlag
	// (#{mouse_sgr_flag}, mode 1006 encoding), MouseStandardFlag
	// (#{mouse_standard_flag}, mode 1000) and MouseUTF8Flag
	// (#{mouse_utf8_flag}, mode 1005 encoding). The three
	// tracking-level flags (any/button/standard) are mutually exclusive
	// in real terminal state -- enabling one clears the others -- but
	// this type reads back whatever tmux currently reports for each
	// without assuming that exclusivity.
	MouseAnyFlag      bool
	MouseButtonFlag   bool
	MouseSGRFlag      bool
	MouseStandardFlag bool
	MouseUTF8Flag     bool
	// WrapFlag is #{wrap_flag}: whether the pane wraps at the right
	// margin (DECAWM, default on).
	WrapFlag bool
	// OriginFlag is #{origin_flag}: whether the cursor is addressed
	// relative to the scroll region (DECOM) rather than the whole pane.
	OriginFlag bool
	// ScrollRegionUpper and ScrollRegionLower are #{scroll_region_upper}
	// and #{scroll_region_lower}: the pane's current DECSTBM scroll
	// region, 0-based row indices, both inclusive.
	ScrollRegionUpper, ScrollRegionLower int
}

// paneSeedStateFormat lists every field PaneSeedState reads, in the exact
// order parsePaneSeedState expects them back, joined by a literal "|"
// that none of these formats can ever themselves emit (they are all
// booleans or bare, unsigned-looking integers).
const paneSeedStateFormat = "#{alternate_on}|#{cursor_x}|#{cursor_y}|#{cursor_flag}|#{insert_flag}|" +
	"#{keypad_cursor_flag}|#{keypad_flag}|" +
	"#{mouse_any_flag}|#{mouse_button_flag}|#{mouse_sgr_flag}|#{mouse_standard_flag}|#{mouse_utf8_flag}|" +
	"#{wrap_flag}|#{origin_flag}|#{scroll_region_upper}|#{scroll_region_lower}"

// paneSeedStateFieldCount is len(strings.Split(paneSeedStateFormat, "|")).
const paneSeedStateFieldCount = 16

// PaneSeedState reads target's full seed state in a single
// `display-message` invocation (PRD II-19; II-20 pairs this with a
// capture-pane call atomically, which is a later task). target may be any
// tmux pane-target string, typically a pane_id.
func (c Client) PaneSeedState(ctx context.Context, target string) (PaneSeedState, error) {
	if c.Socket == "" {
		return PaneSeedState{}, errors.New("tmux socket name is required")
	}
	if target == "" {
		return PaneSeedState{}, errors.New("tmux pane target is required")
	}
	output, err := c.run(ctx, "display-message", "-p", "-t", target, paneSeedStateFormat)
	if err != nil {
		return PaneSeedState{}, fmt.Errorf("read pane seed state for %q: %w", target, err)
	}
	state, err := parsePaneSeedState(strings.TrimRight(string(output), "\n"))
	if err != nil {
		return PaneSeedState{}, fmt.Errorf("parse pane seed state for %q: %w", target, err)
	}
	return state, nil
}

// parsePaneSeedState decodes one display-message line produced by
// paneSeedStateFormat into a PaneSeedState.
func parsePaneSeedState(line string) (PaneSeedState, error) {
	fields := strings.Split(line, "|")
	if len(fields) != paneSeedStateFieldCount {
		return PaneSeedState{}, fmt.Errorf("pane seed state format returned %d fields, want %d: %q", len(fields), paneSeedStateFieldCount, line)
	}
	ints := make([]int, paneSeedStateFieldCount)
	for i, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return PaneSeedState{}, fmt.Errorf("pane seed state field %d (%q): %w", i, f, err)
		}
		ints[i] = n
	}
	return PaneSeedState{
		AlternateOn:       ints[0] != 0,
		CursorX:           ints[1],
		CursorY:           ints[2],
		CursorFlag:        ints[3] != 0,
		InsertFlag:        ints[4] != 0,
		KeypadCursorFlag:  ints[5] != 0,
		KeypadFlag:        ints[6] != 0,
		MouseAnyFlag:      ints[7] != 0,
		MouseButtonFlag:   ints[8] != 0,
		MouseSGRFlag:      ints[9] != 0,
		MouseStandardFlag: ints[10] != 0,
		MouseUTF8Flag:     ints[11] != 0,
		WrapFlag:          ints[12] != 0,
		OriginFlag:        ints[13] != 0,
		ScrollRegionUpper: ints[14],
		ScrollRegionLower: ints[15],
	}, nil
}
