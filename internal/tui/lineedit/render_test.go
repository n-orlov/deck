package lineedit

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

const (
	rev    = "\x1b[7m"
	revOff = "\x1b[27m"
	selBg  = "\x1b[48;2;1;2;3m"
)

var reversedSpan = regexp.MustCompile("\x1b\\[7m(.*?)\x1b\\[27m")

// caretCell returns the text of the single reversed cell in out.
func caretCell(t *testing.T, out string) string {
	t.Helper()
	m := reversedSpan.FindAllStringSubmatch(out, -1)
	if len(m) != 1 {
		t.Fatalf("want exactly one SGR 7 caret cell in %q, got %d", out, len(m))
	}
	return m[0][1]
}

// shown is out without SGR, and the clip marks on each side.
func shown(out string, st Style) (inner string, left, right bool) {
	plain := ansi.Strip(out)
	mark := st.mark()
	if strings.HasPrefix(plain, mark) {
		left = true
		plain = strings.TrimPrefix(plain, mark)
	}
	if strings.HasSuffix(plain, mark) {
		right = true
		plain = strings.TrimSuffix(plain, mark)
	}
	return plain, left, right
}

func TestViewCaretIsReversedBlankAtEnd(t *testing.T) {
	got := New("abc").View(10, Style{})
	if want := "abc" + rev + " " + revOff; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	e, _ := New("abc").Update(key("left"))
	if got, want := e.View(10, Style{}), "ab"+rev+"c"+revOff; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := New("").View(5, Style{}); got != rev+" "+revOff {
		t.Fatalf("empty field: got %q", got)
	}
}

func TestViewBlurredDrawsNoCaret(t *testing.T) {
	if got := New("abc").View(10, Style{Blurred: true}); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestViewNoColorStillCarriesSGR7AtCaret(t *testing.T) {
	// NO_COLOR: the owner passes no selection sequence. The caret is still a
	// reverse-video cell, offered or not.
	for _, e := range []Editor{New("hello"), NewOffered("hello")} {
		out := e.View(20, Style{})
		if strings.Contains(out, "\x1b[48") || strings.Contains(out, "\x1b[38") {
			t.Fatalf("NO_COLOR render carries a colour: %q", out)
		}
		if got := caretCell(t, out); got != " " {
			t.Fatalf("caret cell at end = %q, want a blank", got)
		}
	}
	e, _ := New("hello").Update(key("home"))
	if got := caretCell(t, e.View(20, Style{})); got != "h" {
		t.Fatalf("caret cell at home = %q", got)
	}
}

func TestViewOfferedValueDrawnInSelectionBackground(t *testing.T) {
	out := NewOffered("/tmp/x").View(20, Style{Selection: selBg})
	want := selBg + "/tmp/x" + "\x1b[49m" + rev + " " + revOff
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// Accepted: no selection any more.
	e, _ := NewOffered("/tmp/x").Update(key("left"))
	if out := e.View(20, Style{Selection: selBg}); strings.Contains(out, selBg) {
		t.Fatalf("accepted value still drawn selected: %q", out)
	}
	// A user's own text is not an offer.
	if out := New("/tmp/x").View(20, Style{Selection: selBg}); strings.Contains(out, selBg) {
		t.Fatalf("typed value drawn selected: %q", out)
	}
}

// longValue is exactly 200 characters of words and separators.
func longValue() string {
	v := strings.Repeat("alpha-beta gamma/delta ", 10)[:200]
	return v
}

// step applies one key and refits, as an owner's Update does.
func step(e Editor, k string, width int, st Style) Editor {
	e, _ = e.Update(key(k))
	return e.Fit(width, st)
}

func TestScrollKeepsCaretVisibleAfterHomeEndAndWordJumps(t *testing.T) {
	for _, st := range []Style{{}, {ASCII: true}} {
		v := longValue()
		if len(v) != 200 {
			t.Fatalf("fixture is %d bytes", len(v))
		}
		const width = 20
		e := New(v).Fit(width, st)
		check := func(name string, wantLeft, wantRight bool) {
			t.Helper()
			out := e.View(width, st)
			if w := ansi.StringWidth(out); w > width {
				t.Fatalf("%s: %d cells wide in a %d-cell field: %q", name, w, width, out)
			}
			_, left, right := shown(out, st)
			if left != wantLeft || right != wantRight {
				t.Fatalf("%s (ascii=%v): marks left=%v right=%v, want %v %v: %q", name, st.ASCII, left, right, wantLeft, wantRight, out)
			}
			cell := caretCell(t, out)
			_, after := e.Split()
			if after == "" {
				if cell != " " {
					t.Fatalf("%s: caret at end should be a blank, got %q", name, cell)
				}
			} else if first, _, _, _ := uniseg.FirstGraphemeClusterInString(after, -1); cell != first {
				t.Fatalf("%s: caret cell %q, want %q", name, cell, first)
			}
		}

		check("end", true, false)
		e = step(e, "home", width, st)
		check("home", false, true)
		// Word jumps from home: into the middle, both sides clipped.
		for i := 0; i < 12; i++ {
			e = step(e, "ctrl+right", width, st)
		}
		check("middle after 12 word jumps", true, true)
		// Jump left over words: still visible, still clipped on both sides.
		for i := 0; i < 3; i++ {
			e = step(e, "ctrl+left", width, st)
		}
		check("middle after word jumps left", true, true)
		// Word jumps all the way to the end reach the right edge, no right mark.
		for i := 0; i < 60; i++ {
			e = step(e, "ctrl+right", width, st)
		}
		check("end after word jumps", true, false)
		for i := 0; i < 60; i++ {
			e = step(e, "ctrl+left", width, st)
		}
		check("home after word jumps", false, true)
		e = step(e, "end", width, st)
		check("end again", true, false)
	}
}

func TestScrollClipMarksUnderASCII(t *testing.T) {
	st := Style{ASCII: true}
	e := New(longValue()).Fit(20, st)
	out := ansi.Strip(e.View(20, st))
	if !strings.HasPrefix(out, "...") || strings.Contains(out, "…") {
		t.Fatalf("ASCII end render %q", out)
	}
	e = step(e, "home", 20, st)
	if out := ansi.Strip(e.View(20, st)); !strings.HasSuffix(out, "...") || strings.Contains(out, "…") {
		t.Fatalf("ASCII home render %q", out)
	}
	if out := ansi.Strip(New(longValue()).Fit(20, Style{}).View(20, Style{})); !strings.HasPrefix(out, "…") {
		t.Fatalf("unicode end render %q", out)
	}
}

func TestScrollEveryCaretPositionIsVisible(t *testing.T) {
	v := longValue()
	for _, st := range []Style{{}, {ASCII: true}} {
		e := New(v).Fit(20, st)
		for i := 0; i <= 200; i++ {
			out := e.View(20, st)
			if ansi.StringWidth(out) > 20 {
				t.Fatalf("caret %d: too wide %q", e.Caret(), out)
			}
			caretCell(t, out) // exactly one
			e = step(e, "left", 20, st)
		}
		// And back to the right.
		for i := 0; i < 200; i++ {
			e = step(e, "right", 20, st)
			caretCell(t, e.View(20, st))
		}
	}
}

// wholeClusters reports whether inner is a run of consecutive clusters of v
// starting and ending on cluster boundaries.
func wholeClusters(v, inner string) bool {
	if inner == "" {
		return true
	}
	bounds := map[int]bool{len(v): true}
	for _, o := range clusters(v) {
		bounds[o] = true
	}
	for from := 0; from+len(inner) <= len(v); from++ {
		if v[from:from+len(inner)] == inner && bounds[from] && bounds[from+len(inner)] {
			return true
		}
	}
	return false
}

func TestWideZWJAndCombiningAreMeasuredInCellsAndNeverSplit(t *testing.T) {
	const zwj = "👩‍💻" // woman technologist: one cluster, two cells
	const combining = "é"
	const cjk = "漢"
	if uniseg.StringWidth(zwj) != 2 || uniseg.StringWidth(cjk) != 2 || uniseg.StringWidth(combining) != 1 {
		t.Fatalf("fixture widths: %d %d %d", uniseg.StringWidth(zwj), uniseg.StringWidth(cjk), uniseg.StringWidth(combining))
	}
	unit := "ab" + cjk + combining + zwj + "c" + cjk + zwj + combining
	v := strings.Repeat(unit, 12)
	for _, st := range []Style{{}, {ASCII: true}} {
		for _, width := range []int{5, 6, 7, 8, 9, 20, 21} {
			if st.ASCII && width < 8 {
				continue // two "..." marks and one wide character need 8 cells
			}
			e := New(v).Fit(width, st)
			n := len(clusters(v))
			for i := 0; i <= n; i++ {
				out := e.View(width, st)
				if w := ansi.StringWidth(out); w > width {
					t.Fatalf("width %d caret %d: %d cells: %q", width, e.Caret(), w, out)
				}
				inner, _, _ := shown(out, st)
				// Remove the blank at the end for the whole-cluster check.
				inner = strings.TrimSuffix(inner, " ")
				if !wholeClusters(v, inner) {
					t.Fatalf("width %d caret %d: window %q splits a cluster", width, e.Caret(), inner)
				}
				cell := caretCell(t, out)
				if _, after := e.Split(); after != "" {
					if first, _, _, _ := uniseg.FirstGraphemeClusterInString(after, -1); cell != first {
						t.Fatalf("caret cell %q, want %q", cell, first)
					}
				}
				e = step(e, "left", width, st)
			}
		}
	}
}

func TestWideCharacterCellMeasurementAtScrollEdge(t *testing.T) {
	// "漢漢漢" is 6 cells: in a 5-cell field the window may hold at most two
	// whole characters (4 cells) plus the end blank would not fit beside them.
	e := New("漢漢漢").Fit(5, Style{})
	out := e.View(5, Style{})
	inner, left, right := shown(out, Style{})
	if !left || right {
		t.Fatalf("marks: left=%v right=%v: %q", left, right, out)
	}
	if got := ansi.StringWidth(out); got > 5 {
		t.Fatalf("%d cells: %q", got, out)
	}
	if strings.Contains(inner, "�") || (strings.TrimSuffix(inner, " ") != "漢" && strings.TrimSuffix(inner, " ") != "漢漢") {
		t.Fatalf("inner %q", inner)
	}
	if got := caretCell(t, out); got != " " {
		t.Fatalf("caret cell %q", got)
	}
}

func TestFitShrinksScrollWhenTextBecomesShort(t *testing.T) {
	e := New(longValue()).Fit(20, Style{})
	e, _ = e.Update(key("ctrl+u"))
	e = e.Fit(20, Style{})
	if out := e.View(20, Style{}); strings.Contains(out, "…") {
		t.Fatalf("empty field still shows a clip mark: %q", out)
	}
}

// A text that fits its field with its caret cell, however close to the edge,
// draws whole with no clip mark: a right mark is only owed when something is
// actually clipped. (A 52-character path with its caret blank in a 54-cell
// field once lost its last character to a "..." the mark did not need.)
func TestViewDrawsWholeTextWhenItExactlyFitsTheFieldWithItsCaret(t *testing.T) {
	text := strings.Repeat("p", 52)
	for _, st := range []Style{{}, {ASCII: true}} {
		for _, width := range []int{53, 54, 60} {
			e := New(text).Fit(width, st)
			inner, left, right := shown(e.View(width, st), st)
			if left || right || inner != text+" " {
				t.Fatalf("ASCII=%v width %d: got %q (left mark %v, right mark %v), want the whole text and its caret blank", st.ASCII, width, inner, left, right)
			}
		}
		// One cell narrower really does clip, and the caret stays visible.
		e := New(text).Fit(52, st)
		if _, left, _ := shown(e.View(52, st), st); !left {
			t.Fatalf("ASCII=%v: 52 cells for 53 needed drew no left clip mark", st.ASCII)
		}
	}
}
