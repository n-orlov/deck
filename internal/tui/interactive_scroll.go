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
	// R133 part 2/cure-01-01-2: interactiveBodyLines (interactive.go, R133
	// part 1) heals m.interactiveScrollOffset back to the offset RenderRows
	// actually used -- clamped against the grid's REAL scrollback length, a
	// bound this function's own two clamps above cannot see -- but that
	// heal lands on a value-receiver copy of the model several stack frames
	// deep inside View(), discarded the instant that render call returns.
	// Storing the bound-only offset above, then handing it straight to
	// healInteractiveScrollOffsetFromRender (below) before returning, runs
	// that exact same real-length clamp on the model Update hands back --
	// the one the next input event actually starts from -- so a stored
	// offset left stale-high past the real scrollback length is caught and
	// healed on THIS return, not merely re-computed and thrown away on the
	// next render. The [0, interactive.ScrollbackMaxLines] clamp above is
	// left exactly as it was; healInteractiveScrollOffsetFromRender only
	// clamps further, the same way RenderRows itself always has.
	m.interactiveScrollOffset = offset
	m = m.healInteractiveScrollOffsetFromRender()
	return m, nil
}

// healInteractiveScrollOffsetFromRender re-derives m.interactiveScrollOffset
// against the grid's REAL current scrollback length -- exactly the clamp
// RenderRows itself applies (interactiveBodyLines' own comment) -- and
// returns the healed copy, so a caller that assigns the result back onto
// the model bubbletea keeps between calls carries the healed value
// forward, not merely a render-local one.
//
// cure-01-01 (interactive_scroll_persist_test.go) first added this heal,
// but ONLY inline inside scrollInteractiveByLines above: correct for a
// scroll command, since scrollInteractiveByLines' own return value IS the
// model the next input starts from, but blind to any OTHER way the real
// scrollback length can change out from under a stored offset that was
// never touched by a scroll at all -- a resize that reseeds the grid with
// a shorter or empty real history, or (interactive_transport = capture)
// a background visible-only reseed the poll loop applies wholesale, both
// change what RenderRows would clamp to without either scroll helper ever
// running. interactiveBodyLines (interactive.go, called from View) heals
// its OWN copy of the model the instant a render happens, but View is a
// value receiver several stack frames up (mainView, renderStackedFrame/
// renderSideBySideFrame, previewBodyLines are too), so that heal never
// reaches the model Update hands back either.
//
// Calling this at the top of Update, for every message while m.interactive
// is true, closes that gap: whatever changed the grid's real scrollback
// length since the last time anything looked, the very next Update call --
// regardless of what triggered it -- re-clamps and stores the result onto
// the model it returns, so the NEXT input event after that starts from the
// healed position, not a stale one.
//
// Guarded exactly like interactiveBodyLines' own grid nil-check: a model
// with m.interactive true but no live grid installed (or a grid installed
// but never given a real emulator -- this package's own minimal test
// fixtures build bare &interactive.Session{} values for that) has nothing
// real to clamp against, so this is a no-op for either case, never a
// blanket zeroing of whatever the field already holds.
func (m Model) healInteractiveScrollOffsetFromRender() Model {
	if !m.interactive || m.interactiveGrid == nil || m.interactiveGrid.Grid() == nil {
		return m
	}
	_, contentHeight := m.previewContentSize()
	if contentHeight <= 0 {
		contentHeight = interactiveMinInnerRows
	}
	_, usedOffset := m.interactiveGrid.RenderRows(m.interactiveScrollOffset, contentHeight)
	m.interactiveScrollOffset = usedOffset
	return m
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
