package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/n-orlov/deck/internal/theme"
)

// §11.3 panel chrome: rounded borders, one column of padding, a single seam
// between the sidebar and the preview, and the DECK_ASCII fallback. This
// file only draws — the shape it draws comes from ComputeLayout (§11.2,
// layout.go); there is exactly one geometry implementation, and this file
// consumes it rather than re-deriving widths.

// boxGlyphs is the one border style used throughout (SPEC §11.3: "no
// mixing"), with the documented ASCII fallback for terminals that need it.
type boxGlyphs struct {
	topLeft, topRight, bottomLeft, bottomRight string
	horizontal, vertical                       string
	// seamTop and seamBottom are the T-junctions where the preview's own
	// border meets the sidebar's border above/below it in side-by-side
	// mode, so the top and bottom edges read as one continuous border
	// with a marked seam rather than a corner butting into a straight
	// line.
	seamTop, seamBottom string
}

func (m Model) box() boxGlyphs {
	if m.settings.ASCII {
		return boxGlyphs{"+", "+", "+", "+", "-", "|", "+", "+"}
	}
	return boxGlyphs{"╭", "╮", "╰", "╯", "─", "│", "┬", "┴"}
}

// borderColor renders a border glyph run in tok's colour (SPEC requirement
// 19/42: "the focused surface's border uses the focus colour"). Task 021
// generalises settings.go's settingsBorderColor pattern to every panel this
// file draws: the sidebar (the main view's one focusable region — a dialog
// replaces the whole screen rather than sharing it with the sidebar, so
// there is never a moment where an unfocused sidebar border needs a
// *different* colour) always passes theme.BorderFocus, the preview (never
// focusable in the main view) always passes theme.Border, and fullBoxTop/
// fullBoxBottom/fullBoxContentLine take an explicit focused bool because
// they draw both roles depending on caller (the stacked layout's sidebar
// box vs. its preview box; every framedDialog, which is always the one
// interactive surface once open).
func (m Model) borderColor(tok theme.Token, s string) string {
	return m.colorToken(tok, s)
}

// ellipsis is the marker used when content is truncated to fit a panel's
// content width. Cell-aware truncation (never splitting a wide glyph) is
// task 019; this is the plain byte/rune truncation task 014 needs to keep
// every panel's right edge column-aligned today.
func (m Model) ellipsis() string {
	return m.glyph("…", "...")
}

// cellWidth is the terminal display width of one rune (0 for combining/
// control runes, 1 for ordinary and ambiguous-width runes such as box-
// drawing glyphs, 2 for East-Asian-Wide runes) — the same notion of width
// a real terminal uses to lay out cells, so every panel/sidebar column
// budget in this file is spent in display cells rather than rune count
// (task 019, SPEC requirement 24: "no wide cell is ever split").
func cellWidth(r rune) int {
	return runewidth.RuneWidth(r)
}

// ansiEscapeLen returns the number of bytes, starting at s[i] (which must
// be the ESC byte 0x1b), occupied by a terminal control sequence, so
// stringWidth/truncateToWidth can skip it as zero display columns while
// still copying every one of its bytes through untouched — a captured
// `tmux capture-pane -e` row's SGR colour codes (review finding: "escape
// bytes counted as columns") must never spend panel columns the way a
// printable rune does, or the border after them shears by exactly the
// escape's byte length. Recognises the two forms deck's coloured/status
// pane bytes actually use: CSI (ESC '[' parameter bytes... one final byte)
// and OSC (ESC ']' ... terminated by BEL or ESC '\'). Any other escape is
// treated as ESC plus the one rune after it, so a sequence this scanner
// does not specifically know still advances by a whole rune rather than
// looping forever on it.
func ansiEscapeLen(s string, i int) int {
	if i >= len(s) || s[i] != 0x1b {
		return 0
	}
	if i+1 >= len(s) {
		return 1
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) {
			j++ // final byte, e.g. 'm' for SGR
		}
		return j - i
	case ']':
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				j++
				break
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				j += 2
				break
			}
			j++
		}
		return j - i
	default:
		_, size := utf8.DecodeRuneInString(s[i+1:])
		return 1 + size
	}
}

// stringWidth is s's total terminal display width in cells, treating any
// CSI/OSC escape sequence (ansiEscapeLen) as zero columns rather than
// spending a column per byte the way runewidth.StringWidth would on the
// raw ESC/'['/parameter/final-byte runes.
func stringWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += ansiEscapeLen(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w += cellWidth(r)
		i += size
	}
	return w
}

// truncateToWidth returns the longest prefix of s whose *visible* display
// width is at most budget, counting only printable runes against budget
// and passing every escape sequence it encounters through byte-for-byte at
// zero cost — so a coloured string's SGR codes survive truncation (and
// still colour whatever visible text remains) instead of being cut off
// mid-sequence or spending columns that belong to the text they decorate.
// A rune that would only partially fit — the case a double-width glyph
// creates when the budget has exactly one column left — is dropped in its
// entirety rather than truncated in half, so the returned prefix's visible
// width is always <= budget and never lands mid-glyph (SPEC requirement
// 24).
func truncateToWidth(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	var out strings.Builder
	width := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			n := ansiEscapeLen(s, i)
			out.WriteString(s[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := cellWidth(r)
		if width+w > budget {
			break
		}
		out.WriteString(s[i : i+size])
		width += w
		i += size
	}
	return out.String()
}

// padToWidth appends single-column space runes to s until its display
// width is exactly width. Callers only ever pass a s whose width is
// already <= width (typically truncateToWidth's own result), so this never
// needs to remove anything — padding a wide-rune-safe prefix can never
// overshoot.
func padToWidth(s string, width int) string {
	w := stringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// padTrunc pads s with trailing spaces to exactly width display columns, or
// truncates it (appending the ellipsis when there is room for one) so every
// content line inside a panel is exactly width columns wide — the border
// after it always lands in the same column, and a double-width glyph that
// would straddle the truncation point is dropped whole rather than split
// (SPEC requirement 24; task 019 — this used to count runes, not cells).
func (m Model) padTrunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	sw := stringWidth(s)
	if sw <= width {
		return s + strings.Repeat(" ", width-sw)
	}
	ellW := stringWidth(m.ellipsis())
	if width <= ellW {
		return padToWidth(truncateToWidth(s, width), width)
	}
	budget := width - ellW
	t := padToWidth(truncateToWidth(s, budget), budget)
	return t + m.ellipsis()
}

// wrapText greedily word-wraps s into lines of at most width runes. A single
// word longer than width is placed on its own (overflowing) line rather than
// split, since padTrunc downstream still truncates it to the panel's column
// budget; wrapping never fabricates a hyphen deck's own copy did not write.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var lines []string
	var cur string
	for _, word := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = word
		case len([]rune(cur))+1+len([]rune(word)) <= width:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

// sidebarTopLine draws the sidebar's top border (left corner + top only,
// SPEC requirement 18: the sidebar never draws its own right border). title
// is embedded right after the corner, e.g. "╭ deck — sessions ────".
func (m Model) sidebarTopLine(width int, title string) string {
	bc := m.box()
	inner := width - 1
	label, remain := m.borderLabel(title, inner)
	return m.borderColor(theme.BorderFocus, bc.topLeft) + label + m.borderColor(theme.BorderFocus, strings.Repeat(bc.horizontal, remain))
}

// sidebarBottomLine draws the sidebar's bottom border (left corner + bottom
// only, same reasoning as sidebarTopLine).
func (m Model) sidebarBottomLine(width int) string {
	bc := m.box()
	return m.borderColor(theme.BorderFocus, bc.bottomLeft) + m.borderColor(theme.BorderFocus, strings.Repeat(bc.horizontal, width-1))
}

// sidebarContentLine draws one content row inside the sidebar: left border,
// one column of padding (SPEC requirement 17), text padded/truncated to fill
// the rest, then a trailing column of padding so sidebar content never
// touches the seam — that column, one past the trailing padding, belongs to
// the preview's left border, the single seam (requirement 18). Only the
// real sidebar (width >= SidebarWidthFloor) reserves this trailing column;
// collapsedStripContentLine below covers the 3-wide collapsed strip, which
// has no spare column to give up.
func (m Model) sidebarContentLine(width int, text string) string {
	bc := m.box()
	return m.borderColor(theme.BorderFocus, bc.vertical) + " " + m.padTrunc(text, width-3) + " "
}

// collapsedStripContentLine draws one content row of the 3-column
// collapsed strip (SPEC requirement 15): left border, one column of
// padding, then a single content column with nothing after it — the
// requirement-17 trailing pad in sidebarContentLine does not apply here
// because the strip's whole width is already spent on the marker; giving
// up a column would leave no room for the » glyph or the attention digits.
func (m Model) collapsedStripContentLine(width int, text string) string {
	bc := m.box()
	return m.borderColor(theme.BorderFocus, bc.vertical) + " " + m.padTrunc(text, width-2)
}

// previewTopLine draws the preview's top border on all sides. When seam is
// true, the left corner is the seam's T-junction (SPEC requirement 18)
// rather than a fresh top-left corner, because the sidebar's own top border
// occupies the row to its left.
func (m Model) previewTopLine(width int, title string, seam bool) string {
	bc := m.box()
	left := bc.topLeft
	if seam {
		left = bc.seamTop
	}
	inner := width - 2
	label, remain := m.borderLabel(title, inner)
	return m.borderColor(theme.Border, left) + label + m.borderColor(theme.Border, strings.Repeat(bc.horizontal, remain)) + m.borderColor(theme.Border, bc.topRight)
}

// previewBottomLine mirrors previewTopLine for the bottom edge.
func (m Model) previewBottomLine(width int, seam bool) string {
	bc := m.box()
	left := bc.bottomLeft
	if seam {
		left = bc.seamBottom
	}
	inner := width - 2
	return m.borderColor(theme.Border, left) + m.borderColor(theme.Border, strings.Repeat(bc.horizontal, inner)) + m.borderColor(theme.Border, bc.bottomRight)
}

// previewContentLine draws one content row inside the preview: left border
// (the seam in side-by-side mode), one column of padding, text, one column
// of padding, right border (SPEC requirement 17).
func (m Model) previewContentLine(width int, text string) string {
	bc := m.box()
	inner := width - 4
	return m.borderColor(theme.Border, bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(theme.Border, bc.vertical)
}

// cropMarker marks a preview row that was cut at the right edge (SPEC
// requirement 23) -- distinct from ellipsis()'s "…"/"...", which truncates
// deck's own copy, because this marks foreign pane output deck did not
// write and is choosing not to reflow. Recorded in
// docs/reports/phase2b1-findings.md per task 018's successCriteria.
func (m Model) cropMarker() string {
	return m.glyph("»", ">")
}

// splitPreviewLines turns a raw capture-pane -e byte capture into one
// string per screen row. tmux's own line endings are bare "\n" (capture-pane
// emulates the terminal itself, so an agent's \r\n or bare \r never survives
// into the capture as a literal byte); the \r\n normalisation here only
// guards fixtures/tests that feed in editor-saved CRLF text files directly,
// as task 018's unit tests do with internal/agent/testdata/preview.
func splitPreviewLines(raw []byte) []string {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// cropPreviewBottomLeft crops a captured pane's screen to the preview
// panel's content dimensions (SPEC requirement 23): anchored bottom-left,
// so when the real pane is taller than the panel the *newest* rows are kept
// (the bottom-most avail rows of the capture, which is always the pane's
// current full screen -- capture-pane never returns fewer than
// realHeight rows), and every row is cropped to contentWidth columns from
// column one, never reflowed. A pane smaller than the panel in either
// dimension is never stretched to fill it: missing rows are left as blank
// lines below the real content (top-anchored within the panel) rather than
// scaling the real rows to cover the gap.
//
// Whenever the real geometry exceeds the panel in either dimension, the
// first returned line states it in "WxH of realWxrealH" form (SPEC's
// "45×22 of 120×40", rendered with an ASCII "x" since deck's own copy in
// this panel already uses one) so the user knows they are looking at a
// window onto a larger pane; a pane that fits entirely carries no such
// line, since there is then no window to name. Lines cut at the right edge
// have their last visible column replaced by cropMarker() -- replaced, not
// appended past contentWidth, so the panel's right border always lands in
// the same column regardless of crop offset (task 019 makes this
// substitution cell-aware so it can never land inside a wide glyph).
func (m Model) cropPreviewBottomLeft(raw []byte, contentWidth, contentHeight, realWidth, realHeight int) []string {
	if contentWidth <= 0 || contentHeight <= 0 {
		return nil
	}
	rows := splitPreviewLines(raw)
	cropped := realWidth > contentWidth || realHeight > contentHeight
	avail := contentHeight
	if cropped {
		avail--
	}
	if avail < 0 {
		avail = 0
	}
	start := 0
	if len(rows) > avail {
		start = len(rows) - avail
	}
	visible := rows[start:]
	lines := make([]string, 0, contentHeight)
	if cropped {
		geom := fmt.Sprintf("%dx%d of %dx%d", contentWidth, contentHeight, realWidth, realHeight)
		lines = append(lines, m.padTrunc(geom, contentWidth))
	}
	for _, row := range visible {
		lines = append(lines, m.cropRow(row, contentWidth))
	}
	blank := strings.Repeat(" ", contentWidth)
	for len(lines) < contentHeight {
		lines = append(lines, blank)
	}
	return lines
}

// cropRow crops a single captured screen row to exactly contentWidth
// display columns, left-anchored (column one), never splitting a
// double-width glyph at either the truncation point or the marker column
// (SPEC requirement 24). A row whose real display width already fits is
// only padded; a row that overflows has its content truncated to
// contentWidth-1 columns (never mid-glyph, via truncateToWidth) and its
// final column set to cropMarker() -- always a fresh, whole column, never
// a substitution into a rune that might be the left half of a wide glyph.
func (m Model) cropRow(row string, contentWidth int) string {
	if contentWidth <= 0 {
		return ""
	}
	if stringWidth(row) <= contentWidth {
		return padToWidth(row, contentWidth)
	}
	marker := m.cropMarker()
	markerW := stringWidth(marker)
	budget := contentWidth - markerW
	if budget <= 0 {
		return padToWidth(truncateToWidth(marker, contentWidth), contentWidth)
	}
	content := padToWidth(truncateToWidth(row, budget), budget)
	return content + marker
}

// borderLabel renders a border title (" title ", clamped to inner columns)
// coloured entirely in theme.Title (SPEC requirement 35: "title (panel
// titles)") -- the width/clamp arithmetic below runs on the plain rune
// slice first, and only the final, already-clamped string is wrapped in
// colour, so the self-resetting escape colorToken adds never counts
// against inner and never disturbs the already-coloured border run
// (colorToken's own trailing reset) it is embedded inside. Returns how
// many columns of plain border glyph remain to fill after it.
func (m Model) borderLabel(title string, inner int) (label string, remain int) {
	if title == "" {
		return "", max(inner, 0)
	}
	full := " " + title + " "
	r := []rune(full)
	if len(r) >= inner {
		if inner <= 0 {
			return "", 0
		}
		return m.colorToken(theme.Title, string(r[:inner])), 0
	}
	return m.colorToken(theme.Title, full), inner - len(r)
}

// fullBoxTop/fullBoxBottom/fullBoxContentLine draw an independent, fully-
// bordered panel's top/bottom/content edges — used for the stacked layout
// mode (§11.2), where the list and preview panels stack vertically rather
// than sharing a vertical seam, so each keeps all four of its own borders,
// and for framedDialog, which wraps every dialog/overlay in the same box.
// focused selects theme.BorderFocus (the stacked layout's sidebar box; any
// framedDialog, always the one interactive surface once open) vs.
// theme.Border (the stacked layout's preview box, never focusable).
func (m Model) fullBoxTop(width int, title string, focused bool) string {
	bc := m.box()
	inner := width - 2
	label, remain := m.borderLabel(title, inner)
	tok := theme.Border
	if focused {
		tok = theme.BorderFocus
	}
	return m.borderColor(tok, bc.topLeft) + label + m.borderColor(tok, strings.Repeat(bc.horizontal, remain)) + m.borderColor(tok, bc.topRight)
}

func (m Model) fullBoxBottom(width int, focused bool) string {
	bc := m.box()
	tok := theme.Border
	if focused {
		tok = theme.BorderFocus
	}
	return m.borderColor(tok, bc.bottomLeft) + m.borderColor(tok, strings.Repeat(bc.horizontal, width-2)) + m.borderColor(tok, bc.bottomRight)
}

func (m Model) fullBoxContentLine(width int, text string, focused bool) string {
	bc := m.box()
	inner := width - 4
	tok := theme.Border
	if focused {
		tok = theme.BorderFocus
	}
	return m.borderColor(tok, bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(tok, bc.vertical)
}

// framedDialog wraps a dialog/overlay's existing free-form text in the same
// rounded/ASCII box used by the main view's panels (SPEC requirement 16:
// "on every panel, dialog and overlay"), sized to the widest existing line
// so none of a dialog's own line-wrapping changes and no existing substring
// assertion moves off the line it was written for. It never truncates —
// several dialog help lines run past a typical terminal's width already and
// still must not lose the substring a unit test asserts on the raw string,
// so the box grows to fit rather than eliding.
func (m Model) framedDialog(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	width := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > width {
			width = w
		}
	}
	boxWidth := width + 4
	out := make([]string, 0, len(lines)+2)
	out = append(out, m.fullBoxTop(boxWidth, "", true))
	for _, line := range lines {
		out = append(out, m.fullBoxContentLine(boxWidth, line, true))
	}
	out = append(out, m.fullBoxBottom(boxWidth, true))
	return strings.Join(out, "\n")
}

// fitLines pads or truncates lines to exactly n entries so every panel's
// content area is filled to its full height regardless of how much real
// content there is.
func fitLines(lines []string, n int) []string {
	if n < 0 {
		n = 0
	}
	if len(lines) >= n {
		return lines[:n]
	}
	out := make([]string, n)
	copy(out, lines)
	return out
}
