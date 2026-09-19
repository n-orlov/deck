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

// interactiveScrollState is the one place PRD II-51's scrollback position
// is actually stored, pointed at by Model.interactiveScroll and therefore
// SHARED by every copy Bubble Tea's value-receiver dance makes of that
// Model.
//
// The sharing is the whole point (R133, cure-01-01-2). The offset a frame
// actually renders with is not knowable outside the render: RenderRows
// (internal/interactive/grid.go) clamps the requested offset against the
// grid's REAL current scrollback length, a bound that changes without
// anything on this side being told -- a resize reseeds the grid with a
// shorter or empty history, and under interactive_transport = capture the
// poll loop replaces it wholesale from a visible-only re-seed. With the
// offset kept as a plain int FIELD, the clamped value RenderRows returned
// could only ever be written onto whichever value-receiver copy of Model
// happened to be on the stack -- View's, mainView's, previewBodyLines',
// interactiveBodyLines' -- every one of them discarded the moment the
// render returned, so the position the NEXT input event started from was
// whatever stale, unclamped number the last Update happened to leave
// behind. Storing it in a cell every copy points at means the heal
// survives a render that is not followed by any Update at all: render
// clamps, the cell holds the clamped value, and the next scroll steps
// from exactly the position the operator was actually looking at.
//
// Read it through Model.interactiveScrollOffset() and write it through
// Model.setInteractiveScrollOffset(); nothing outside those two touches
// the field.
type interactiveScrollState struct {
	offset int
}

// interactiveScrollOffset reports the stored scrollback position: 0 is the
// live bottom, a positive value is that many lines back into the grid's
// own bounded scrollback. A Model with no cell installed at all (a bare
// Model{} literal, as some of this package's own minimal test fixtures
// build) has never scrolled anywhere, so it reads as the live bottom
// rather than panicking.
func (m Model) interactiveScrollOffset() int {
	if m.interactiveScroll == nil {
		return 0
	}
	return m.interactiveScroll.offset
}

// setInteractiveScrollOffset stores n as the scrollback position. The
// receiver is a pointer purely so a Model that has no cell yet can be
// given one (New installs one for every real model; a bare Model{} test
// fixture has none) -- the STORE itself goes through the shared cell, so
// it is visible to every other copy of the same model, including the
// caller of whichever value-receiver method did the write. That is
// exactly what makes a render's own heal outlive the render
// (interactiveScrollState's doc above).
func (m *Model) setInteractiveScrollOffset(n int) {
	if m.interactiveScroll == nil {
		m.interactiveScroll = &interactiveScrollState{}
	}
	m.interactiveScroll.offset = n
}

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
	// Step from the offset rendering actually USED, not from whatever was
	// requested last: healInteractiveScrollOffsetFromRender re-clamps the
	// stored position against the grid's real current scrollback length
	// first, so "one line further back" after a resize or a visible-only
	// reseed wiped the history out is offset 1 -- one line back from the
	// live bottom the operator is genuinely looking at -- and never
	// `stale + 1` collapsing onto whatever new maximum happens to exist by
	// then (cure-01-01-2's second half).
	m = m.healInteractiveScrollOffsetFromRender()
	offset := m.interactiveScrollOffset() + delta
	if offset < 0 {
		offset = 0
	}
	if offset > interactive.ScrollbackMaxLines {
		offset = interactive.ScrollbackMaxLines
	}
	// R133 part 2/cure-01-01-2: interactiveBodyLines (interactive.go, R133
	// part 1) heals the stored offset back to the offset RenderRows
	// actually used -- clamped against the grid's REAL scrollback length, a
	// bound this function's own two clamps above cannot see. Storing the
	// bound-only offset above, then handing it straight to
	// healInteractiveScrollOffsetFromRender (below) before returning, runs
	// that exact same real-length clamp right now rather than waiting for
	// the next render, so a request past the real length is stored as the
	// position it will actually be shown at. The
	// [0, interactive.ScrollbackMaxLines] clamp above is left exactly as it
	// was; healInteractiveScrollOffsetFromRender only clamps further, the
	// same way RenderRows itself always has.
	m.setInteractiveScrollOffset(offset)
	m = m.healInteractiveScrollOffsetFromRender()
	return m, nil
}

// healInteractiveScrollOffsetFromRender re-derives the stored scroll
// offset against the grid's REAL current scrollback length -- exactly the
// clamp RenderRows itself applies (interactiveBodyLines' own comment) --
// and returns the healed model. Because the offset lives in a shared cell
// (interactiveScrollState above), the healed value is visible to every
// copy of this Model, not merely to whoever assigns the return value.
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
// running.
//
// Two further call sites close that gap. interactiveBodyLines'/mainView's
// own store of RenderRows' used offset now lands in the shared cell, so a
// plain RENDER heals the stored position even when no Update follows it at
// all; and calling this at the top of Update, for every message while
// m.interactive is true, catches a length change that nothing has
// rendered since.
//
// Guarded exactly like interactiveBodyLines' own grid nil-check: a model
// with m.interactive true but no live grid installed (or a grid installed
// but never given a real emulator -- this package's own minimal test
// fixtures build bare &interactive.Session{} values for that) has nothing
// real to clamp against, so this is a no-op for either case, never a
// blanket zeroing of whatever the position already holds.
func (m Model) healInteractiveScrollOffsetFromRender() Model {
	if !m.interactive || m.interactiveGrid == nil || m.interactiveGrid.Grid() == nil {
		return m
	}
	_, contentHeight := m.previewContentSize()
	if contentHeight <= 0 {
		contentHeight = interactiveMinInnerRows
	}
	_, usedOffset := m.interactiveGrid.RenderRows(m.interactiveScrollOffset(), contentHeight)
	m.setInteractiveScrollOffset(usedOffset)
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
