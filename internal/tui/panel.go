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
// 19/42/44: "the focused surface's border uses the focus colour"). Task 021
// generalised settings.go's settingsBorderColor pattern to every panel this
// file draws: originally the sidebar (the main view's one focusable region
// — a dialog replaces the whole screen rather than sharing it with the
// sidebar, so there was never a moment where an unfocused sidebar border
// needed a *different* colour) always passed theme.BorderFocus and the
// preview always passed theme.Border. Task 063/II-44 adds interactive
// mode as the ONE other place focus can live in the main view: while
// m.interactive is true every keystroke forwards to the live pane the
// preview renders instead of driving the sidebar, so the two panels swap
// which token they draw in — see previewFocused/sidebarBorderToken/
// previewBorderToken below, which every sidebar*Line/preview*Line function
// now calls instead of a hardcoded token. fullBoxTop/fullBoxBottom/
// fullBoxContentLine keep taking an explicit focused bool because they
// draw both roles depending on caller (the stacked layout's sidebar box
// vs. its preview box, both now driven by the same previewFocused split;
// every framedDialog, which is always the one interactive surface once
// open and always passes focused=true regardless of m.interactive).
func (m Model) borderColor(tok theme.Token, s string) string {
	return m.colorToken(tok, s)
}

// previewFocused reports whether the preview panel, rather than the
// sidebar, currently owns the main view's one unit of keyboard focus (SPEC
// requirement 44). Interactive mode (m.interactive, task 061 onward) is
// the ONLY way that ever happens — outside it the sidebar is unconditionally
// the main view's one focusable region (requirement 42's own reasoning,
// which is also why `tab` stays unbound there).
func (m Model) previewFocused() bool {
	return m.interactive
}

// sidebarBorderToken/previewBorderToken resolve the two panels' own border
// token from previewFocused, so every sidebar*Line/preview*Line function
// below draws whichever token currently applies instead of one hardcoded
// at task-021 time (before interactive mode existed, focus could never
// leave the sidebar in the main view).
func (m Model) sidebarBorderToken() theme.Token {
	if m.previewFocused() {
		return theme.Border
	}
	return theme.BorderFocus
}

func (m Model) previewBorderToken() theme.Token {
	if m.previewFocused() {
		return theme.BorderFocus
	}
	return theme.Border
}

// mainViewOverlayActive reports whether some overlay currently owns the
// keyboard while mainView keeps rendering underneath it (task 025's `t`
// theme picker and task 123's `/` filter text field, per their own file
// comments -- theme_picker.go: "the real sidebar/preview panels (mainView)
// keep rendering throughout"; filter.go: "mainView keeps rendering ...
// filtering never replaces View()"). Both intercept every keystroke before
// it reaches either panel's own bindings (see the dispatch order in
// tui.go's Update), so while either is active neither the sidebar nor the
// preview panel is the one actually holding focus, even though both still
// render on screen. Every other overlay (m.help, m.creating, m.settingsOpen,
// m.eventLogOpen, ...) replaces View() outright (see View()'s early-return
// chain), so mainView -- and this seam -- never renders under them at all.
func (m Model) mainViewOverlayActive() bool {
	return m.themePicking || m.filtering
}

// seamBorderToken resolves the single shared column between the sidebar
// and the preview panel (SPEC requirement 57's amendment: the seam and its
// ┬/┭ T-junctions read as focused whenever EITHER panel is -- unlike
// sidebarBorderToken/previewBorderToken, which each report their own
// panel's mutually-exclusive focus state, the seam sits on the boundary
// between both and belongs to whichever side currently holds focus. The
// one case neither side does -- some overlay atop mainView owns the
// keyboard instead (mainViewOverlayActive) -- is the one case the seam
// reads as plain border, matching a dialog's own unfocused border
// elsewhere in this package. Only this one column and its T-junctions use
// this rule; the rest of the preview's own border (and all of the
// sidebar's) keeps using previewBorderToken/sidebarBorderToken exactly as
// before -- glyph ownership (the preview draws every seam glyph) is
// unchanged.
func (m Model) seamBorderToken() theme.Token {
	if m.mainViewOverlayActive() {
		return theme.Border
	}
	return theme.BorderFocus
}

// sidebarSelectionToken is the sidebar's selected-row background token
// (SPEC requirement 42/44): `selection` while the sidebar itself holds
// focus, `selection_idle` — the existing token internal/theme/token.go:19
// already declares, no new token needed — while focus has moved to the
// preview via interactive mode. Mirrors settings.go's
// settingsSelectionToken, which resolves the identical two-token split for
// the settings takeover's own pair of focusable lists.
func (m Model) sidebarSelectionToken() theme.Token {
	if m.previewFocused() {
		return theme.SelectionIdle
	}
	return theme.Selection
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
//
// SPEC §11.3 ("every truncated coloured run re-emits its own reset"):
// dropping the visible runes that no longer fit must never also drop an
// open BACKGROUND span's own closing reset that happened to sit past the
// cut point in s — a highlighted row's background left open across the
// truncation would otherwise bleed into whatever the caller concatenates
// next (the ellipsis, padding, or the seam/border beyond the panel's own
// width; see settingsRenderRow, which opens one background span and closes
// it with a single trailing reset only at the very end of the composed
// line, long after any mid-line truncation here would have cut it off).
// backgroundSpanTracker below watches only background-setting/-clearing
// SGR parameters as they pass through untouched; if a span is still open
// once this function stops advancing — whether because the budget ran out
// or because s simply ended without ever closing it — a synthetic
// "\x1b[0m" is appended so the returned string always leaves the terminal
// in a closed background state, at zero visible cost. Foreground-only runs
// (no background ever opened) are untouched: escape_width_test.go's
// TestTruncateToWidthKeepsEscapeBytesItPassesOver deliberately exercises a
// foreground-only span truncated mid-run and asserts NO reset is
// synthesised there — dropping a foreground colour's own trailing reset
// changes only the colour of text that already stopped being emitted
// (whatever comes next in the same coloured run resolves its own
// foreground itself, panel.go's colorToken/bgColorToken and
// settingsRenderRow always emit one attribute-scoped SGR before their
// text), never the shape of another panel's surface the way an open
// background does.
func truncateToWidth(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	var out strings.Builder
	var bg backgroundSpanTracker
	width := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			n := ansiEscapeLen(s, i)
			esc := s[i : i+n]
			bg.observe(esc)
			out.WriteString(esc)
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
	if bg.open {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// backgroundSpanTracker watches the SGR (CSI ... 'm') escape sequences
// truncateToWidth passes through and reports whether the run currently
// sits inside an open BACKGROUND span — a background colour set (ANSI
// 40-47/100-107, or the extended "48;5;N"/"48;2;R;G;B" forms bgSgrForToken
// emits, see theme_color.go) that has not since been cleared by either a
// bare background reset (49) or a full reset (0, or an SGR escape with no
// parameters at all, which the standard treats identically to 0). Only
// background state is tracked (see truncateToWidth's doc comment for why
// foreground is deliberately left alone); other SGR attributes (bold,
// underline, foreground) are parsed only far enough to skip their own
// extended parameters correctly so a background code sharing one escape
// with them is not misread.
type backgroundSpanTracker struct {
	open bool
}

func (b *backgroundSpanTracker) observe(esc string) {
	if len(esc) < 3 || esc[1] != '[' || esc[len(esc)-1] != 'm' {
		return // not an SGR sequence (e.g. an OSC title, or a cursor move)
	}
	params := esc[2 : len(esc)-1]
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		code := fields[i]
		if code == "" || code == "0" {
			b.open = false
			continue
		}
		n, err := parseSGRCode(code)
		if err != nil {
			continue
		}
		switch {
		case n == 49:
			b.open = false
		case n >= 40 && n <= 47:
			b.open = true
		case n >= 100 && n <= 107:
			b.open = true
		case n == 48:
			b.open = true
			// Extended colour: "48;5;N" (one more param) or
			// "48;2;R;G;B" (three more) -- skip them so they are never
			// mistaken for their own top-level SGR codes.
			if i+1 < len(fields) {
				switch fields[i+1] {
				case "5":
					i += 2
				case "2":
					i += 4
				}
			}
		case n == 38:
			// Extended foreground colour, same shape as 48 above; skip
			// its params too so an accompanying 48 later in the same
			// escape is not misaligned.
			if i+1 < len(fields) {
				switch fields[i+1] {
				case "5":
					i += 2
				case "2":
					i += 4
				}
			}
		}
	}
}

// parseSGRCode parses one semicolon-separated SGR parameter as a decimal
// integer, without pulling in strconv's full surface for what is always a
// short run of ASCII digits from an escape sequence deck itself generated.
func parseSGRCode(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a digit: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
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
// wrapText greedily word-wraps s into lines of at most width DISPLAY
// COLUMNS (task 030 -- this used to count runes; a coloured line's SGR
// escape bytes inflated its perceived width, making a coloured dialog line
// wrap far sooner than an identical uncoloured one). stringWidth already
// treats every CSI/OSC escape sequence as zero columns, and strings.Fields
// only ever splits on whitespace bytes, never inside an escape sequence, so
// a "word" that carries a self-contained colorToken span (SGR...text...
// reset) stays intact and displays correctly wherever it lands.
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
		case stringWidth(cur)+1+stringWidth(word) <= width:
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
	tok := m.sidebarBorderToken()
	return m.borderColor(tok, bc.topLeft) + label + m.borderColor(tok, strings.Repeat(bc.horizontal, remain))
}

// sidebarBottomLine draws the sidebar's bottom border (left corner + bottom
// only, same reasoning as sidebarTopLine).
func (m Model) sidebarBottomLine(width int) string {
	bc := m.box()
	tok := m.sidebarBorderToken()
	return m.borderColor(tok, bc.bottomLeft) + m.borderColor(tok, strings.Repeat(bc.horizontal, width-1))
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
	return m.borderColor(m.sidebarBorderToken(), bc.vertical) + " " + m.padTrunc(text, width-3) + " "
}

// collapsedStripContentLine draws one content row of the 3-column
// collapsed strip (SPEC requirement 15): left border, one column of
// padding, then a single content column with nothing after it — the
// requirement-17 trailing pad in sidebarContentLine does not apply here
// because the strip's whole width is already spent on the marker; giving
// up a column would leave no room for the » glyph or the attention digits.
func (m Model) collapsedStripContentLine(width int, text string) string {
	bc := m.box()
	return m.borderColor(m.sidebarBorderToken(), bc.vertical) + " " + m.padTrunc(text, width-2)
}

// previewTopLine draws the preview's top border on all sides. When seam is
// true, the left corner is the seam's T-junction (SPEC requirement 18)
// rather than a fresh top-left corner, because the sidebar's own top border
// occupies the row to its left.
func (m Model) previewTopLine(width int, title string, seam bool) string {
	bc := m.box()
	left := bc.topLeft
	leftTok := m.previewBorderToken()
	if seam {
		left = bc.seamTop
		leftTok = m.seamBorderToken()
	}
	inner := width - 2
	label, remain := m.borderLabel(title, inner)
	tok := m.previewBorderToken()
	return m.borderColor(leftTok, left) + label + m.borderColor(tok, strings.Repeat(bc.horizontal, remain)) + m.borderColor(tok, bc.topRight)
}

// previewBottomLine mirrors previewTopLine for the bottom edge.
func (m Model) previewBottomLine(width int, seam bool) string {
	bc := m.box()
	left := bc.bottomLeft
	leftTok := m.previewBorderToken()
	if seam {
		left = bc.seamBottom
		leftTok = m.seamBorderToken()
	}
	inner := width - 2
	tok := m.previewBorderToken()
	return m.borderColor(leftTok, left) + m.borderColor(tok, strings.Repeat(bc.horizontal, inner)) + m.borderColor(tok, bc.bottomRight)
}

// previewContentLine draws one content row inside the preview: left border
// (the seam in side-by-side mode -- coloured by seamBorderToken, the
// shared either-panel-focused rule, not previewBorderToken), one column of
// padding, text, one column of padding, right border coloured by
// previewBorderToken as always (SPEC requirement 17).
func (m Model) previewContentLine(width int, text string) string {
	bc := m.box()
	inner := width - 4
	return m.borderColor(m.seamBorderToken(), bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(m.previewBorderToken(), bc.vertical)
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
// focused selects theme.BorderFocus vs. theme.Border for whichever role the
// caller is drawing. Every framedDialog call always passes focused=true
// (a dialog is always the one interactive surface once open). The stacked
// layout's own two calls (renderStackedFrame) pass !m.previewFocused()/
// m.previewFocused() respectively (task 063/II-44) — before interactive
// mode existed the sidebar box was unconditionally true and the preview
// box unconditionally false, since focus could never leave the sidebar.
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

// dialogWidth is every §11.4 dialog/overlay's box width (SPEC.md:1070,
// task 030): 80% of the viewport, clamped to [26, 80] columns. At the
// documented supported minimum viewport (80 columns, §11), this resolves to
// 64; it only reaches the 80 ceiling once the viewport is 100 columns or
// wider. The lower clamp and the take-the-full-viewport fallback below it
// are best-effort only on a below-minimum terminal (SPEC.md:1071-1074) --
// not a size this phase carries test obligations for -- so a viewport
// narrower than the clamp itself is handed back as-is rather than padded
// out to 26.
func (m Model) dialogWidth() int {
	viewport, _ := m.frameSize()
	w := viewport * 80 / 100
	switch {
	case w > 80:
		w = 80
	case w < 26:
		if viewport < 26 {
			return viewport
		}
		w = 26
	}
	return w
}

// framedDialog wraps a dialog/overlay's existing free-form text in the same
// rounded/ASCII box used by the main view's panels (SPEC requirement 16:
// "on every panel, dialog and overlay"), at dialogWidth's fixed viewport-
// derived width rather than growing to fit the widest existing line (task
// 030 -- SPEC.md:1070 states the box's width is a function of the
// viewport, not of the dialog's own content). A content line wider than
// the box's inner budget wraps at a word boundary (wrapText, ANSI-aware via
// stringWidth) instead of being truncated away -- several dialog help
// lines carry a reason a truncated line would lose, so this never elides a
// substring in favour of keeping one physical line.
func (m Model) framedDialog(body string) string {
	boxWidth := m.dialogWidth()
	lines := m.wrapDialogLines(body)
	out := make([]string, 0, len(lines)+2)
	out = append(out, m.fullBoxTop(boxWidth, "", true))
	for _, line := range lines {
		out = append(out, m.fullBoxContentLine(boxWidth, line, true))
	}
	out = append(out, m.fullBoxBottom(boxWidth, true))
	return strings.Join(out, "\n")
}

// wrapDialogLines is framedDialog/framedDialogScrollable's shared
// body-to-lines step: split the raw body into physical lines, word-
// wrapping (ANSI-aware, via stringWidth/wrapText) any line wider than the
// box's own inner width instead of ever truncating it away.
func (m Model) wrapDialogLines(body string) []string {
	inner := m.dialogWidth() - 4
	if inner < 1 {
		inner = 1
	}
	rawLines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	var lines []string
	for _, line := range rawLines {
		if stringWidth(line) <= inner {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, wrapText(line, inner)...)
	}
	return lines
}

// dialogContentBudget is framedDialogScrollable's own content-row budget:
// the frame height minus the box's own top/bottom border -- the most rows
// a scrollable overlay's body may ever render at once without pushing the
// whole dialog past the frame budget (task 078, requirement 39 residual).
func (m Model) dialogContentBudget() int {
	_, frameHeight := m.frameSize()
	budget := frameHeight - 2
	if budget < 1 {
		budget = 1
	}
	return budget
}

// dialogMaxScroll is the largest scroll offset framedDialogScrollable will
// ever honour for this body: zero once the wrapped body already fits
// inside dialogContentBudget, otherwise the number of lines hanging off
// the bottom of one full page. Callers use this to clamp a stored scroll
// offset without re-deriving framedDialogScrollable's own slicing.
func (m Model) dialogMaxScroll(body string) int {
	lines := m.wrapDialogLines(body)
	budget := m.dialogContentBudget()
	if len(lines) <= budget {
		return 0
	}
	return len(lines) - budget
}

// dialogScrollBy is PgUp/PgDn's own step for a scrollable overlay (task
// 078): one full dialogContentBudget page, dir<0 up/dir>0 down, clamped to
// [0, dialogMaxScroll(body)] so repeated PgDown past the bottom (or PgUp
// past the top) cannot inflate the stored offset past what the very next
// render would ever show.
func (m Model) dialogScrollBy(current int, body string, dir int) int {
	next := current + dir*m.dialogContentBudget()
	if next < 0 {
		next = 0
	}
	if max := m.dialogMaxScroll(body); next > max {
		next = max
	}
	return next
}

// framedDialogScrollable is framedDialog's height-bounded counterpart
// (task 078, requirement 39 residual): the `?` help overlay, `E` event
// log and `i` detail view are the only widgets on screen while open (no
// footer, no sidebar underneath), so unlike every other §11.4 dialog --
// bounded by its own field count -- their content can grow far past the
// frame budget (helpText alone is 273 lines at 80x24). Rather than
// truncate (SPEC requirement 39: pagination/scrolling, never silently
// dropped content), the body is clipped to a scrollable window: scroll
// (clamped here against the body's own dialogMaxScroll, so a caller need
// not pre-clamp) selects which wrapped line is topmost. Below the frame
// budget its output is byte-for-byte what framedDialog would have
// produced -- clipping only ever engages once content actually overflows.
func (m Model) framedDialogScrollable(body string, scroll int) string {
	boxWidth := m.dialogWidth()
	lines := m.wrapDialogLines(body)
	budget := m.dialogContentBudget()
	visible := lines
	if len(lines) > budget {
		max := len(lines) - budget
		if scroll < 0 {
			scroll = 0
		}
		if scroll > max {
			scroll = max
		}
		visible = lines[scroll : scroll+budget]
	}
	out := make([]string, 0, len(visible)+2)
	out = append(out, m.fullBoxTop(boxWidth, "", true))
	for _, line := range visible {
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
