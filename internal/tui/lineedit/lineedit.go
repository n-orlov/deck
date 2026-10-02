// Package lineedit is deck's one line editor (SPEC.md §11.11): a pure value of
// text, a caret and an "offered" flag, edited with readline's keys. A character
// is a grapheme cluster, a word is a run of letters and digits, and a paste is
// one insertion with control characters dropped. The package draws nothing and
// holds no widths; rendering belongs to the caller.
package lineedit

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

// Editor is the editing model. The zero value is an empty, unoffered field.
// Every method returns a new Editor and leaves the receiver untouched.
type Editor struct {
	text    string
	caret   int // byte offset, always on a grapheme-cluster boundary
	offered bool
	scroll  int // byte offset of the first drawn cluster, set by Fit
}

// New returns an editor holding text with the caret at its end. The text is
// the user's own, so it is not offered.
func New(text string) Editor {
	text = StripControl(text)
	return Editor{text: text, caret: len(text)}
}

// NewOffered returns an editor holding a value the field starts with but the
// user has not chosen (§11.11): whole text selected, caret at its end.
func NewOffered(text string) Editor {
	e := New(text)
	e.offered = text != ""
	return e
}

// Value is the field's text.
func (e Editor) Value() string { return e.text }

// Caret is the caret's byte offset into Value.
func (e Editor) Caret() int { return e.caret }

// Offered reports whether the whole text is still an unaccepted offer.
func (e Editor) Offered() bool { return e.offered }

// Split returns the text before and after the caret.
func (e Editor) Split() (before, after string) { return e.text[:e.caret], e.text[e.caret:] }

// StripControl drops every control character (newline, carriage return and tab
// included) from s.
func StripControl(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// Paste is one bracketed paste at the caret, control characters dropped. While
// the value is offered a paste replaces it wholesale (§11.11), even a paste
// that is empty once its control characters are dropped: the offer is then
// replaced by an empty, accepted value.
func (e Editor) Paste(s string) Editor {
	if e.offered {
		s = StripControl(s)
		return Editor{text: s, caret: len(s)}
	}
	return e.Insert(s)
}

// Insert is one insertion of typed text at the caret, control characters
// dropped. While the value is offered it replaces the whole text. Typed text
// that is empty once control characters are dropped is not a printable
// keystroke, so it changes nothing and an offer survives it.
func (e Editor) Insert(s string) Editor {
	s = StripControl(s)
	if s == "" {
		return e
	}
	if e.offered {
		return Editor{text: s, caret: len(s)}
	}
	e.text = e.text[:e.caret] + s + e.text[e.caret:]
	e.caret += len(s)
	return e
}

// Update applies one key message. handled is false for a key the editor does
// not own (navigation keys, a key the §11.11 table does not list, and alt+w,
// which is a copy and so the caller's); the editor is then returned unchanged,
// and an offer is not accepted by a key that is not the editor's.
func (e Editor) Update(msg tea.KeyMsg) (Editor, bool) {
	if msg.Paste {
		return e.Paste(string(msg.Runes)), true
	}
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace:
		if msg.Alt {
			return e.alt(msg)
		}
		return e.Insert(string(msg.Runes)), true
	}
	switch msg.String() {
	case "left", "ctrl+b":
		return e.accept().moveTo(e.prevBoundary()), true
	case "right", "ctrl+f":
		return e.accept().moveTo(e.nextBoundary()), true
	case "home", "ctrl+a":
		return e.accept().moveTo(0), true
	case "end", "ctrl+e":
		return e.accept().moveTo(len(e.text)), true
	case "ctrl+left":
		return e.accept().moveTo(e.wordLeft()), true
	case "ctrl+right":
		return e.accept().moveTo(e.wordRight()), true
	case "backspace", "ctrl+h":
		a := e.accept()
		return a.deleteRange(a.prevBoundary(), a.caret), true
	case "delete", "ctrl+d":
		a := e.accept()
		return a.deleteRange(a.caret, a.nextBoundary()), true
	case "ctrl+w":
		a := e.accept()
		return a.deleteRange(a.whitespaceLeft(), a.caret), true
	case "alt+backspace":
		a := e.accept()
		return a.deleteRange(a.wordLeft(), a.caret), true
	case "ctrl+u":
		a := e.accept()
		return a.deleteRange(0, a.caret), true
	case "ctrl+k":
		a := e.accept()
		return a.deleteRange(a.caret, len(a.text)), true
	}
	return e, false
}

func (e Editor) alt(msg tea.KeyMsg) (Editor, bool) {
	switch string(msg.Runes) {
	case "b":
		return e.accept().moveTo(e.wordLeft()), true
	case "f":
		return e.accept().moveTo(e.wordRight()), true
	}
	return e, false
}

func (e Editor) accept() Editor {
	e.offered = false
	return e
}

func (e Editor) moveTo(pos int) Editor {
	e.caret = pos
	return e
}

func (e Editor) deleteRange(from, to int) Editor {
	e.text = e.text[:from] + e.text[to:]
	e.caret = from
	return e
}

// clusters returns the start offsets of every grapheme cluster of s.
func clusters(s string) []int {
	var starts []int
	off := 0
	for off < len(s) {
		starts = append(starts, off)
		cl, _, _, _ := uniseg.FirstGraphemeClusterInString(s[off:], -1)
		off += len(cl)
	}
	return starts
}

func (e Editor) prevBoundary() int {
	starts := clusters(e.text[:e.caret])
	if len(starts) == 0 {
		return 0
	}
	return starts[len(starts)-1]
}

func (e Editor) nextBoundary() int {
	if e.caret >= len(e.text) {
		return len(e.text)
	}
	cl, _, _, _ := uniseg.FirstGraphemeClusterInString(e.text[e.caret:], -1)
	return e.caret + len(cl)
}

// isWord reports whether a cluster is part of a word: it starts with a letter
// or digit. A combining mark or joiner is carried inside its cluster.
func isWord(cluster string) bool {
	for _, r := range cluster {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return false
}

func isSpace(cluster string) bool {
	for _, r := range cluster {
		return unicode.IsSpace(r)
	}
	return false
}

// back walks left from the caret over clusters matching skip, then over those
// matching run, and returns the offset reached.
func (e Editor) back(skip, run func(string) bool) int {
	starts := clusters(e.text[:e.caret])
	i := len(starts)
	end := e.caret
	cluster := func(i int) string {
		hi := e.caret
		if i+1 < len(starts) {
			hi = starts[i+1]
		}
		return e.text[starts[i]:hi]
	}
	for i > 0 && skip(cluster(i-1)) {
		i--
	}
	for i > 0 && run(cluster(i-1)) {
		i--
	}
	if i == len(starts) {
		return end
	}
	return starts[i]
}

func not(f func(string) bool) func(string) bool {
	return func(s string) bool { return !f(s) }
}

// wordLeft is the start of the word before the caret: non-word characters are
// skipped first, then the run of letters and digits.
func (e Editor) wordLeft() int { return e.back(not(isWord), isWord) }

// whitespaceLeft is bash's unix-word-rubout boundary: whitespace is skipped,
// then the run of non-whitespace.
func (e Editor) whitespaceLeft() int { return e.back(isSpace, not(isSpace)) }

// wordRight is the end of the word after the caret: non-word characters are
// skipped first, then the run of letters and digits.
func (e Editor) wordRight() int {
	off := e.caret
	next := func() (string, bool) {
		if off >= len(e.text) {
			return "", false
		}
		cl, _, _, _ := uniseg.FirstGraphemeClusterInString(e.text[off:], -1)
		return cl, true
	}
	for {
		cl, ok := next()
		if !ok || isWord(cl) {
			break
		}
		off += len(cl)
	}
	for {
		cl, ok := next()
		if !ok || !isWord(cl) {
			break
		}
		off += len(cl)
	}
	return off
}

// FirstCluster splits s into its first grapheme cluster and the rest.
func FirstCluster(s string) (first, rest string) {
	first, rest, _, _ = uniseg.FirstGraphemeClusterInString(s, -1)
	return first, rest
}
