package lineedit

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Style is how a caller wants an editor drawn. The editor itself never reads
// the environment or the theme: the owner passes what it resolved.
type Style struct {
	// ASCII draws the clip marks as "..." instead of "…" (DECK_ASCII).
	ASCII bool
	// Selection is the SGR open sequence for the theme's `selection`
	// background, drawn behind an offered value. Empty (NO_COLOR, or no
	// theme) draws the offered text plain.
	Selection string
	// Blurred leaves the caret out: only the focused field draws one.
	Blurred bool
}

const (
	sgrReverse      = "\x1b[7m"
	sgrReverseOff   = "\x1b[27m"
	sgrDefaultBgOff = "\x1b[49m"
)

func (s Style) mark() string {
	if s.ASCII {
		return "..."
	}
	return "…"
}

func (s Style) markWidth() int {
	if s.ASCII {
		return 3
	}
	return 1
}

// item is one drawn unit: a grapheme cluster, or the reversed blank that
// stands for the caret at the end of the text.
type item struct {
	text  string
	off   int // byte offset of text in the value
	width int
}

// items lays the value out in cells. The caret's own item is returned as an
// index; at the end of the text it is a trailing blank.
func (e Editor) items() (its []item, caretIdx int) {
	for _, off := range clusters(e.text) {
		cl, _, _, _ := uniseg.FirstGraphemeClusterInString(e.text[off:], -1)
		its = append(its, item{text: cl, off: off, width: uniseg.StringWidth(cl)})
	}
	caretIdx = len(its)
	for i, it := range its {
		if it.off == e.caret {
			caretIdx = i
			break
		}
	}
	if e.caret >= len(e.text) {
		its = append(its, item{off: len(e.text), width: 1})
		caretIdx = len(its) - 1
	}
	return its, caretIdx
}

// window returns the end of the run of whole items that fit in width cells
// when drawing starts at item s, marks included.
func window(its []item, s, width int, mw int) int {
	avail := width
	if s > 0 {
		avail -= mw
	}
	end := s
	used := 0
	rest := 0 // cells the items from end onward need, drawn without a right mark
	for _, it := range its[s:] {
		rest += it.width
	}
	for end < len(its) {
		if rest <= avail-used {
			return len(its) // everything left fits: no right mark is needed
		}
		w := its[end].width
		room := avail - used
		if end+1 < len(its) {
			room -= mw // a right mark will be needed unless this is the last
		}
		if w > room {
			// The last item may still fit without a right mark.
			if end == len(its)-1 && w <= avail-used {
				return len(its)
			}
			break
		}
		used += w
		rest -= w
		end++
	}
	if end == s && s < len(its) {
		end = s + 1 // never draw an empty window: the item overflows its cell
	}
	return end
}

// Fit returns the editor with its horizontal scroll adjusted so the caret is
// inside a width-cell field. A scroll edge is always a cluster boundary, so a
// wide character, a ZWJ emoji or a combining sequence is never cut. Owners
// call Fit when the caret moves or the field resizes and keep the result, so
// the window does not jump while the caret stays inside it.
func (e Editor) Fit(width int, st Style) Editor {
	its, caret := e.items()
	s := fitScroll(its, caret, e.scrollItem(its), width, st.markWidth())
	e.scroll = its[s].off
	return e
}

// scrollItem is the index of the first item at or after the stored scroll.
func (e Editor) scrollItem(its []item) int {
	for i, it := range its {
		if it.off >= e.scroll {
			return i
		}
	}
	return len(its) - 1
}

func fitScroll(its []item, caret, s, width, mw int) int {
	if width <= 0 {
		return s
	}
	total := 0
	for _, it := range its {
		total += it.width
	}
	if total <= width {
		return 0
	}
	if s > caret {
		s = caret
	}
	for s < caret && window(its, s, width, mw) <= caret {
		s++
	}
	for s > 0 && window(its, s-1, width, mw) == len(its) {
		s--
	}
	return s
}

// View draws the editor in width cells: the visible run of the value from the
// scroll offset, the caret as a reverse-video cell (a reversed blank at the
// end of the text), `…` (`...` under ASCII) on each clipped side, and an
// offered value in the theme's selection background. The result is at most
// width cells wide. The caret is SGR 7 whatever the colour settings, so it
// survives NO_COLOR.
func (e Editor) View(width int, st Style) string {
	if width <= 0 {
		return ""
	}
	its, caret := e.items()
	mw := st.markWidth()
	s := fitScroll(its, caret, e.scrollItem(its), width, mw)
	end := window(its, s, width, mw)

	var b strings.Builder
	if s > 0 {
		b.WriteString(st.mark())
	}
	offered := e.offered && st.Selection != "" && len(e.text) > 0
	writeCells(&b, its[s:end], caret-s, st, offered)
	if end < len(its) {
		b.WriteString(st.mark())
	}
	return b.String()
}

// writeCells writes the visible items, caret being the caret's index within
// them: the caret cell reversed, an offered value in the selection
// background, closing the background after the last cell.
func writeCells(b *strings.Builder, its []item, caret int, st Style, offered bool) {
	inSel := false
	for i, it := range its {
		isCaret := i == caret && !st.Blurred
		inSel = writeSelectionEdge(b, st, inSel, offered && it.text != "" && !isCaret)
		writeCell(b, it, isCaret)
	}
	if inSel {
		b.WriteString(sgrDefaultBgOff)
	}
}

// writeSelectionEdge opens the selection background when the next cell wants
// it and none is open, closes it when the next cell does not, and returns
// whether the background is open afterwards.
func writeSelectionEdge(b *strings.Builder, st Style, inSel, want bool) bool {
	if want && !inSel {
		b.WriteString(st.Selection)
		return true
	}
	if !want && inSel {
		b.WriteString(sgrDefaultBgOff)
		return false
	}
	return inSel
}

// writeCell writes one item, reversed when it is the caret cell (the end
// cell, which has no text, as a reversed space).
func writeCell(b *strings.Builder, it item, isCaret bool) {
	switch {
	case isCaret && it.text == "":
		b.WriteString(sgrReverse + " " + sgrReverseOff)
	case isCaret:
		b.WriteString(sgrReverse + it.text + sgrReverseOff)
	default:
		b.WriteString(it.text)
	}
}
