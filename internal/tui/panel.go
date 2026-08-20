package tui

import "strings"

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

// borderColor is the focus indicator for a panel's border (SPEC requirement
// 19: "the focused surface's border uses the focus colour"). Phase 2b-1 has
// no theme system yet (that lands in Phase 2b-2's §11.6), and the sidebar is
// the only focusable region the main view ever draws — a dialog replaces the
// whole screen rather than sharing it with the sidebar, so there is never a
// moment where an unfocused sidebar border needs a *different* colour, only
// a moment where it is not drawn at all. Reusing the existing NO_COLOR/
// DECK_COLOR-aware styling keeps that one visible cue real rather than
// inventing a second colour knob ahead of the theme system that will own it.
func (m Model) borderColor(s string) string {
	return m.color(s)
}

// ellipsis is the marker used when content is truncated to fit a panel's
// content width. Cell-aware truncation (never splitting a wide glyph) is
// task 019; this is the plain byte/rune truncation task 014 needs to keep
// every panel's right edge column-aligned today.
func (m Model) ellipsis() string {
	return m.glyph("…", "...")
}

// padTrunc pads s with trailing spaces to exactly width runes, or truncates
// it (appending the ellipsis when there is room for one) so every content
// line inside a panel is exactly width columns — the border after it always
// lands in the same column.
func (m Model) padTrunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s + strings.Repeat(" ", width-len(r))
	}
	ell := []rune(m.ellipsis())
	if width <= len(ell) {
		return string(r[:width])
	}
	return string(r[:width-len(ell)]) + m.ellipsis()
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
	return m.borderColor(bc.topLeft) + label + m.borderColor(strings.Repeat(bc.horizontal, remain))
}

// sidebarBottomLine draws the sidebar's bottom border (left corner + bottom
// only, same reasoning as sidebarTopLine).
func (m Model) sidebarBottomLine(width int) string {
	bc := m.box()
	return m.borderColor(bc.bottomLeft) + m.borderColor(strings.Repeat(bc.horizontal, width-1))
}

// sidebarContentLine draws one content row inside the sidebar: left border,
// one column of padding (SPEC requirement 17), then text padded/truncated to
// fill the rest — there is no right border to pad against, since that
// column belongs to the preview's left border, the single seam (requirement
// 18).
func (m Model) sidebarContentLine(width int, text string) string {
	bc := m.box()
	return m.borderColor(bc.vertical) + " " + m.padTrunc(text, width-2)
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
	return m.borderColor(left) + label + m.borderColor(strings.Repeat(bc.horizontal, remain)) + m.borderColor(bc.topRight)
}

// previewBottomLine mirrors previewTopLine for the bottom edge.
func (m Model) previewBottomLine(width int, seam bool) string {
	bc := m.box()
	left := bc.bottomLeft
	if seam {
		left = bc.seamBottom
	}
	inner := width - 2
	return m.borderColor(left) + m.borderColor(strings.Repeat(bc.horizontal, inner)) + m.borderColor(bc.bottomRight)
}

// previewContentLine draws one content row inside the preview: left border
// (the seam in side-by-side mode), one column of padding, text, one column
// of padding, right border (SPEC requirement 17).
func (m Model) previewContentLine(width int, text string) string {
	bc := m.box()
	inner := width - 4
	return m.borderColor(bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(bc.vertical)
}

// borderLabel renders a border title (" title ", plain/uncoloured so it
// never disturbs an already-coloured border run) clamped to inner columns,
// and returns how many columns of plain border glyph remain to fill after
// it.
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
		return string(r[:inner]), 0
	}
	return full, inner - len(r)
}

// sidebarTitle is the sidebar's top-border title. "deck" keeps the exact
// existing colour styling (m.color("deck")) so an already-passing assertion
// on that specific coloured substring keeps meaning what it always meant;
// only the border glyphs around it are newly coloured as the focus cue
// (SPEC requirement 19).
func (m Model) sidebarTitleLine(width int) string {
	bc := m.box()
	inner := width - 1
	visible := " deck" + m.glyph(" — ", " - ") + "sessions "
	label := " " + m.color("deck") + m.glyph(" — ", " - ") + "sessions "
	remain := inner - len([]rune(visible))
	if remain < 0 {
		remain = 0
	}
	return m.borderColor(bc.topLeft) + label + m.borderColor(strings.Repeat(bc.horizontal, remain))
}

// fullBoxTop/fullBoxBottom draw an independent, fully-bordered panel's top
// and bottom edges — used for the stacked layout mode (§11.2), where the
// list and preview panels stack vertically rather than sharing a vertical
// seam, so each keeps all four of its own borders.
func (m Model) fullBoxTop(width int, title string) string {
	bc := m.box()
	inner := width - 2
	label, remain := m.borderLabel(title, inner)
	return m.borderColor(bc.topLeft) + label + m.borderColor(strings.Repeat(bc.horizontal, remain)) + m.borderColor(bc.topRight)
}

func (m Model) fullBoxBottom(width int) string {
	bc := m.box()
	return m.borderColor(bc.bottomLeft) + m.borderColor(strings.Repeat(bc.horizontal, width-2)) + m.borderColor(bc.bottomRight)
}

func (m Model) fullBoxContentLine(width int, text string) string {
	bc := m.box()
	inner := width - 4
	return m.borderColor(bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(bc.vertical)
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
	out = append(out, m.fullBoxTop(boxWidth, ""))
	for _, line := range lines {
		out = append(out, m.fullBoxContentLine(boxWidth, line))
	}
	out = append(out, m.fullBoxBottom(boxWidth))
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
