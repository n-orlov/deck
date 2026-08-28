package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	osc52 "github.com/aymanbagabas/go-osc52/v2"

	"github.com/n-orlov/deck/internal/theme"
)

// oscClipboardWriter is where the drag-to-copy selection's best-effort
// OSC 52 half (SPEC §11.8: "an OSC 52 write additionally reaches the
// user's system clipboard ... best-effort by nature") writes its escape
// sequence -- os.Stdout, the SAME real terminal Bubble Tea's own
// renderer writes to (cmd/deck/main.go never overrides tea.WithOutput),
// in production. A test swaps it out so the exact bytes can be asserted
// without a real terminal attached.
var oscClipboardWriter io.Writer = os.Stdout

// writeOSCClipboardBestEffort writes text to the outer terminal's system
// clipboard via OSC 52 (SPEC §11.8): deliberately best-effort and
// deliberately unchecked -- whether it actually reaches a real clipboard
// depends on the outer terminal and, if deck's own controlling terminal
// is itself a tmux client, on THAT tmux's own `set-clipboard` option,
// neither of which deck can detect or control, so there is no error this
// call could report that would mean anything actionable. It is never the
// only thing a copy does: SetSelectionBuffer (the load-bearing, tested
// half) always runs first regardless of this call's outcome.
func writeOSCClipboardBestEffort(text string) {
	_, _ = osc52.New(text).WriteTo(oscClipboardWriter)
}

// previewCellAt resolves one absolute terminal cell (as tea.MouseMsg
// reports it, 0-indexed) to the interactive preview's own
// content-relative (col, row) -- the exact coordinate space
// internal/interactive's RenderRows/SelectedText/AbsoluteRow all use --
// through the SAME banner/layout accounting hitTest (mouse.go) already
// applies for every other mouse gesture, never a second geometry
// computation (SPEC §11.8's own hit-testing rule). ok is false for
// anything outside the preview's own content box: off-frame, the
// sidebar, the seam, or the border/padding cells
// previewContentLine/fullBoxContentLine draw around the content itself.
func (m Model) previewCellAt(x, y int) (col, row int, ok bool) {
	if x < 0 || y < 0 {
		return 0, 0, false
	}
	width, _ := m.frameSize()
	banner := len(m.startupBanner(width)) + len(m.themeBanner(width))
	frameY := y - banner
	if frameY < 0 {
		return 0, 0, false
	}
	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		return m.previewCellAtStacked(layout, x, frameY)
	}
	return m.previewCellAtSideBySide(layout, x, frameY)
}

// previewCellAtSideBySide mirrors hitTestSideBySide's own accounting
// (mouse.go): previewContentLine draws left-border, one padding column,
// content, one padding column, right border, so content starts two
// columns past the panel's own left edge -- which sits exactly at the
// sidebar's own width, since the seam/left-border character occupies
// that column. Width/height come from m.previewContentSize() itself --
// the SAME box previewBodyLines/interactiveBodyLines actually render
// into -- rather than a second, independently-derived formula that could
// silently drift from it.
func (m Model) previewCellAtSideBySide(layout LayoutResult, x, y int) (col, row int, ok bool) {
	sw, height := layout.Sidebar.Width, layout.Sidebar.Height
	if y < 0 || y >= height || x <= sw {
		return 0, 0, false
	}
	contentWidth, contentHeight := m.previewContentSize()
	contentRow := y - 1
	if contentRow < 0 || contentRow >= contentHeight {
		return 0, 0, false
	}
	contentCol := x - sw - 2
	if contentCol < 0 || contentCol >= contentWidth {
		return 0, 0, false
	}
	return contentCol, contentRow, true
}

// previewCellAtStacked mirrors hitTestStacked's own accounting: the
// preview panel is a fully-bordered box (fullBoxContentLine) below the
// sidebar's own box, with the identical border+pad+content+pad+border
// structure as the side-by-side preview. Width/height again come from
// m.previewContentSize(), never re-derived.
func (m Model) previewCellAtStacked(layout LayoutResult, x, y int) (col, row int, ok bool) {
	lh := layout.Sidebar.Height
	if lh >= 2 {
		y -= lh
	}
	ph := layout.Preview.Height
	if ph < 2 || y <= 0 || y >= ph-1 || x <= 0 {
		return 0, 0, false
	}
	contentWidth, contentHeight := m.previewContentSize()
	contentCol := x - 2
	contentRow := y - 1
	if contentCol < 0 || contentCol >= contentWidth || contentRow < 0 || contentRow >= contentHeight {
		return 0, 0, false
	}
	return contentCol, contentRow, true
}

// previewClampToContent answers the same question as previewCellAt, but
// for a motion event that may have run past the preview's own edge
// (SPEC §11.8's seam-drag already grants that continuity; a text
// selection drag gets the same treatment rather than freezing the moment
// the pointer leaves the box): it clamps to the nearest still-in-bounds
// cell instead of refusing.
func (m Model) previewClampToContent(x, y int) (col, row int) {
	width, _ := m.frameSize()
	banner := len(m.startupBanner(width)) + len(m.themeBanner(width))
	frameY := y - banner
	layout := m.computeLayout()
	contentWidth, contentHeight := m.previewContentSize()
	var rawCol, rawRow int
	if layout.Effective == LayoutStacked {
		if lh := layout.Sidebar.Height; lh >= 2 {
			frameY -= lh
		}
		rawCol = x - 2
		rawRow = frameY - 1
	} else {
		rawCol = x - layout.Sidebar.Width - 2
		rawRow = frameY - 1
	}
	return clampToRange(rawCol, 0, contentWidth-1), clampToRange(rawRow, 0, contentHeight-1)
}

func clampToRange(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// beginInteractiveSelection starts a drag-to-copy selection (steer 017
// item 3/task 216, SPEC §11.8's "a drag beginning inside the preview
// selects") when a left-button press resolves inside the interactive
// preview's own content box while m.interactive is true. ok is false
// (and m unchanged) for every other case, including a press over the
// PASSIVE preview: the passive preview is a capture-pane crop with no
// underlying Grid to extract cell-exact plain text from, so drag-to-copy
// is deliberately scoped to interactive mode only (recorded as a delta
// against SPEC.md's own broader "the preview" wording in
// docs/reports/phase3d-spec-deltas.md's item 3 entry) -- the caller
// (tui.go's Update, the tea.MouseMsg case's own m.interactive branch)
// falls through to the ordinary "a click over the preview does nothing"
// no-op when ok is false.
func (m Model) beginInteractiveSelection(x, y int) (Model, bool) {
	if !m.interactive || m.interactiveGrid == nil {
		return m, false
	}
	col, row, ok := m.previewCellAt(x, y)
	if !ok {
		return m, false
	}
	m.interactiveSelecting = true
	m.interactiveSelectDragged = false
	m.interactiveSelectAnchorCol, m.interactiveSelectAnchorRow = col, row
	m.interactiveSelectCurrentCol, m.interactiveSelectCurrentRow = col, row
	return m, true
}

// updateInteractiveSelection extends an in-progress selection to
// wherever the drag's latest motion event landed (previewClampToContent,
// so a drag that runs off the panel's own edge keeps selecting to the
// nearest still-in-bounds cell rather than stopping). It is a no-op
// (returns m unchanged) unless m.interactiveSelecting is already true --
// the caller (tui.go's Update) only reaches this once press
// has already confirmed the drag began inside the preview's content box.
func (m Model) updateInteractiveSelection(x, y int) Model {
	if !m.interactiveSelecting {
		return m
	}
	col, row := m.previewClampToContent(x, y)
	m.interactiveSelectDragged = true
	m.interactiveSelectCurrentCol, m.interactiveSelectCurrentRow = col, row
	return m
}

// commitInteractiveSelection is release's own job (tui.go's Update):
// once a genuine drag (interactiveSelectDragged -- a plain
// click, press+release with no intervening motion, commits nothing, so
// "a click over the preview does nothing" (SPEC §11.8) stays true) has
// happened, it converts the anchor/current VIEW-relative cells into the
// grid's own absolute row space via interactiveGrid.AbsoluteRow, keyed
// to the SAME interactiveScrollOffset the drag was actually performed
// against (read once, up front, rather than re-read after extraction in
// case a concurrent render already advanced it), extracts the plain text
// of the linear run between them (interactive.Session.SelectedText),
// writes it to deck's OWN tmux buffer (SPEC §11.8's load-bearing, tested
// half -- tmux.Client.SetSelectionBuffer) and makes a best-effort OSC 52
// write alongside it (the deliberately untested half). Selection state
// is always cleared on return, whether or not the tmux write succeeded:
// a failed copy must not leave deck thinking a selection is still
// "in progress" for the very next press.
func (m Model) commitInteractiveSelection() Model {
	anchorCol, anchorRow := m.interactiveSelectAnchorCol, m.interactiveSelectAnchorRow
	curCol, curRow := m.interactiveSelectCurrentCol, m.interactiveSelectCurrentRow
	dragged := m.interactiveSelectDragged
	grid := m.interactiveGrid
	offset := m.interactiveScrollOffset
	client := m.tmuxClient

	m.interactiveSelecting = false
	m.interactiveSelectDragged = false

	if !dragged || grid == nil {
		return m
	}
	_, contentHeight := m.previewContentSize()
	fromRow := grid.AbsoluteRow(offset, contentHeight, anchorRow)
	toRow := grid.AbsoluteRow(offset, contentHeight, curRow)
	text := grid.SelectedText(anchorCol, fromRow, curCol, toRow)

	ctx := context.Background()
	if err := client.SetSelectionBuffer(ctx, text); err != nil {
		m.attachError = "Cannot copy selection: " + err.Error()
		return m
	}
	writeOSCClipboardBestEffort(text)
	m.attachError = ""
	return m
}

// selectionCloseSGR is the background-only reset (SGR 49, "default
// background colour") highlightInProgressSelection/highlightRangeSGR pair
// with theme.Selection's own opening sequence: closing with a bare reset
// (\x1b[0m) would also clear whatever FOREGROUND colour the pane's own
// content already carries at that point in the row (the agent's own
// syntax-highlighting, say), which a selection highlight must leave alone
// -- only the background changes while a drag is in progress. Built via
// fmt.Sprintf (never a written-out digit-literal escape sequence) for the
// reason theme_color.go's own dynamic SGR construction is: this package's
// TestNoColorLiterals forbids a hardcoded non-zero SGR code, since 49 is
// not itself a colour selection but the guard cannot tell that from a
// digit literal alone.
var selectionCloseSGR = fmt.Sprintf("\x1b[%dm", 49)

// highlightInProgressSelection marks the drag-to-copy selection's cells
// (SPEC §11.8: "From the press until the release, the selected cells are
// marked with the selection token ... and the marking clears when the
// release commits the copy") while a drag is in progress
// (m.interactiveSelecting -- set true by beginInteractiveSelection on
// press, set false by commitInteractiveSelection on release, so this is
// automatically a no-op the instant a release commits the copy, with no
// separate clearing step of its own). It resolves the selection's
// per-row column range through the SAME m.interactiveScrollOffset and
// interactiveGrid.AbsoluteRow conversion commitInteractiveSelection uses
// (via interactive.Session.SelectionHighlightRange, AbsoluteRow's own
// sibling) -- never a second, independently derived row space -- and
// applies theme.Selection as a BACKGROUND-only span (selectionCloseSGR)
// over exactly those columns of each already-rendered row, leaving
// whatever foreground colour the pane's own content carries untouched.
// It is a no-op (rows returned unchanged) once the drag ends, if there is
// no grid to resolve AbsoluteRow against, or if colour is disabled
// (NO_COLOR / DECK_COLOR=0), matching colorToken's own gating -- a
// selection with no visible marking is still a selection, just an
// invisible one under those settings, exactly like every other themed
// surface in this package.
func (m Model) highlightInProgressSelection(lines []string, contentHeight int) []string {
	if !m.interactiveSelecting || m.interactiveGrid == nil {
		return lines
	}
	openSeq, ok := m.backgroundSGR(theme.Selection)
	if !ok {
		return lines
	}
	grid := m.interactiveGrid
	offset := m.interactiveScrollOffset
	fromCol, anchorRow := m.interactiveSelectAnchorCol, m.interactiveSelectAnchorRow
	toCol, curRow := m.interactiveSelectCurrentCol, m.interactiveSelectCurrentRow
	fromRow := grid.AbsoluteRow(offset, contentHeight, anchorRow)
	toRow := grid.AbsoluteRow(offset, contentHeight, curRow)

	out := make([]string, len(lines))
	for i, line := range lines {
		startCol, endCol, sel := grid.SelectionHighlightRange(offset, contentHeight, i, fromCol, fromRow, toCol, toRow)
		if !sel {
			out[i] = line
			continue
		}
		out[i] = highlightRangeSGR(line, startCol, endCol, openSeq, selectionCloseSGR)
	}
	return out
}

// highlightRangeSGR wraps the display columns [startCol, endCol]
// (inclusive) of one already-rendered preview row in openSeq/closeSeq,
// walking the row exactly like panel.go's stringWidth/truncateToWidth do
// (every CSI/OSC escape byte passed through untouched at zero columns via
// ansiEscapeLen, every printable rune's own cellWidth spent against the
// running column count) so the insertion point never lands mid-escape or
// mid-glyph. A span still open when the row ends is closed there, so a
// highlight reaching a row's own last column never bleeds into whatever
// the caller appends next (padTrunc's own padding, the panel border).
func highlightRangeSGR(line string, startCol, endCol int, openSeq, closeSeq string) string {
	if startCol > endCol {
		return line
	}
	var out strings.Builder
	col := 0
	opened := false
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			n := ansiEscapeLen(line, i)
			out.WriteString(line[i : i+n])
			i += n
			continue
		}
		switch {
		case !opened && col >= startCol && col <= endCol:
			out.WriteString(openSeq)
			opened = true
		case opened && col > endCol:
			out.WriteString(closeSeq)
			opened = false
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		out.WriteString(line[i : i+size])
		col += cellWidth(r)
		i += size
	}
	if opened {
		out.WriteString(closeSeq)
	}
	return out.String()
}
