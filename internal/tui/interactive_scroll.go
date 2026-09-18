package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// interactiveWheelStepLines is PRD II-51's wheel granularity: a single
// notch moves a few lines, matching the small, incremental step most
// terminal emulators already give their OWN native scrollback for one
// notch (handleMouse's sidebar wheel handling uses the same "a few rows
// per notch" convention for the same reason).
const interactiveWheelStepLines = 3

// scrollInteractiveByLines adjusts the interactive grid's own scroll
// offset (PRD II-51) by delta lines: positive moves further back into
// scrollback, negative moves toward the live bottom. It clamps to
// [0, interactive.ScrollbackMaxLines] itself -- the bound the grid can
// hold at all -- but RenderRows (internal/interactive/grid.go) clamps
// AGAIN against the grid's ACTUAL scrollback length on every call, so an
// offset this clamp lets through while the grid holds fewer lines than
// that bound is silently satisfied by whatever is really available rather
// than showing blank rows above real content.
//
// How full the scrollback is at any moment is not something this side can
// assume either way (issue #29). The ENTRY seed pulls up to
// ScrollbackMaxLines of the pane's own tmux history
// (internal/interactive.CaptureSeedWithHistory, called from
// enterInteractiveBody), so a pane that had been running for an hour can
// be scrollable to the bound from the very first keypress; but the same
// pane freshly started has almost nothing, an alternate-screen pane has
// nothing at all by design, a capture that could not be taken atomically
// degrades to no history, and under interactive_transport = capture the
// first poll tick replaces the grid wholesale from a visible-only
// re-seed. RenderRows' own clamp is what makes all of those cases behave
// the same here, which is why nothing in this file inspects the length.
func (m Model) scrollInteractiveByLines(delta int) (tea.Model, tea.Cmd) {
	if !m.interactive || m.interactiveGrid == nil {
		return m, nil
	}
	offset := m.interactiveScrollOffset + delta
	if offset < 0 {
		offset = 0
	}
	if offset > interactive.ScrollbackMaxLines {
		offset = interactive.ScrollbackMaxLines
	}
	m.interactiveScrollOffset = offset
	return m, nil
}

// scrollInteractiveByPage is Shift+PgUp/PgDn's own step (PRD II-51): a
// whole page, matching plain PgUp/PgDn's own convention everywhere else
// this codebase forwards it byte-for-byte to the target instead (see
// interactiveNamedKey). dir > 0 moves back into scrollback, dir < 0
// toward the live bottom. The page height is previewContentSize's own
// content height -- the exact height interactiveBodyLines renders into --
// so a resize never leaves this stepping by a stale row count.
func (m Model) scrollInteractiveByPage(dir int) (tea.Model, tea.Cmd) {
	if !m.interactive || m.interactiveGrid == nil {
		return m, nil
	}
	_, height := m.previewContentSize()
	if height <= 0 {
		height = interactiveMinInnerRows
	}
	if dir < 0 {
		height = -height
	}
	return m.scrollInteractiveByLines(height)
}

// shiftPgUpCSIString/shiftPgDownCSIString name the raw CSI bytes a real
// xterm-class terminal sends for Shift+PgUp/PgDn: "\x1b[5;2~"/"\x1b[6;2~"
// (parameter 2 is xterm's own "Shift" modifier code; PgUp/PgDn's own bare
// forms without a trailing ";2" are "\x1b[5~"/"\x1b[6~", which
// interactiveNamedKey already forwards to the target by name). Bubble
// Tea v1.3.10's own key-sequence table (vendored key_sequences.go/key.go)
// has no entry for either of the Shift-modified forms at all -- unlike
// the Shift-modified arrow keys, which DO get their own KeyShiftUp/Down/
// Left/Right types, PgUp/PgDn have no Shift-modified Key type whatsoever
// (confirmed by grepping the vendored source for "KeyShiftPgUp": nothing)
// -- so a real terminal's bytes for these two fall through Bubble Tea's
// decoder (key.go's detectOneMsg -> detectSequence -> the exact-match
// table, then the unknownCSIRe fallback) into unknownCSISequenceMsg, a
// type outside this package's own module and unexported, so it cannot be
// named in a type switch here at all.
//
// unknownCSISequenceMsg DOES satisfy fmt.Stringer, formatted by its own
// String() method (key.go) as fmt.Sprintf("?CSI%+v?", raw-bytes-after-
// the-leading-"\x1b[") -- deterministic for this exact, pinned bubbletea
// version (go.mod pins it), and the only handle this package has on a
// message type it cannot otherwise recognise short of forking Bubble Tea
// itself. Built with fmt.Sprintf using the SAME format string bubbletea's
// own String() uses, rather than a hand-written literal, so a change to
// that formatting (in a version bump) breaks visibly instead of quietly
// mismatching.
var (
	shiftPgUpCSIString   = fmt.Sprintf("?CSI%+v?", []byte("5;2~"))
	shiftPgDownCSIString = fmt.Sprintf("?CSI%+v?", []byte("6;2~"))
)

// shiftPageScrollDir reports which of Shift+PgUp (dir=1) / Shift+PgDn
// (dir=-1) msg names, via the fmt.Stringer fallback the two CSI strings
// above document. Update (tui.go) calls this from its own top-level
// switch's default case: a message not matching either string here is
// left completely alone, exactly as it already was before this existed
// (every message type not otherwise handled by that switch already falls
// through to a plain "return m, nil", so adding this default case
// changes nothing for anything else it might also happen to match).
func shiftPageScrollDir(msg tea.Msg) (dir int, ok bool) {
	s, isStringer := msg.(fmt.Stringer)
	if !isStringer {
		return 0, false
	}
	switch s.String() {
	case shiftPgUpCSIString:
		return 1, true
	case shiftPgDownCSIString:
		return -1, true
	}
	return 0, false
}
