package lineedit

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	case "ctrl+left":
		return tea.KeyMsg{Type: tea.KeyCtrlLeft}
	case "ctrl+right":
		return tea.KeyMsg{Type: tea.KeyCtrlRight}
	case "alt+backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+b":
		return tea.KeyMsg{Type: tea.KeyCtrlB}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+e":
		return tea.KeyMsg{Type: tea.KeyCtrlE}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "ctrl+h":
		return tea.KeyMsg{Type: tea.KeyCtrlH}
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+w":
		return tea.KeyMsg{Type: tea.KeyCtrlW}
	case "alt+b":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}
	case "alt+f":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f"), Alt: true}
	case "alt+w":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w"), Alt: true}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func paste(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

// at builds an editor over text with the caret after the first n bytes of it.
func at(text string, n int) Editor {
	e := New(text)
	e.caret = n
	return e
}

func TestKeyTable(t *testing.T) {
	cases := []struct {
		name  string
		from  Editor
		key   string
		value string
		caret int
	}{
		// caret left / right
		{"left middle", at("abcd", 2), "left", "abcd", 1},
		{"left start", at("abcd", 0), "left", "abcd", 0},
		{"left end", at("abcd", 4), "left", "abcd", 3},
		{"left empty", New(""), "left", "", 0},
		{"ctrl+b middle", at("abcd", 2), "ctrl+b", "abcd", 1},
		{"ctrl+b start", at("abcd", 0), "ctrl+b", "abcd", 0},
		{"right middle", at("abcd", 2), "right", "abcd", 3},
		{"right start", at("abcd", 0), "right", "abcd", 1},
		{"right end", at("abcd", 4), "right", "abcd", 4},
		{"right empty", New(""), "right", "", 0},
		{"ctrl+f middle", at("abcd", 2), "ctrl+f", "abcd", 3},
		{"ctrl+f end", at("abcd", 4), "ctrl+f", "abcd", 4},
		// home / end
		{"home middle", at("abcd", 2), "home", "abcd", 0},
		{"home start", at("abcd", 0), "home", "abcd", 0},
		{"home end", at("abcd", 4), "home", "abcd", 0},
		{"home empty", New(""), "home", "", 0},
		{"ctrl+a middle", at("abcd", 2), "ctrl+a", "abcd", 0},
		{"end middle", at("abcd", 2), "end", "abcd", 4},
		{"end start", at("abcd", 0), "end", "abcd", 4},
		{"end end", at("abcd", 4), "end", "abcd", 4},
		{"end empty", New(""), "end", "", 0},
		{"ctrl+e middle", at("abcd", 2), "ctrl+e", "abcd", 4},
		// backspace / delete
		{"backspace middle", at("abcd", 2), "backspace", "acd", 1},
		{"backspace start", at("abcd", 0), "backspace", "abcd", 0},
		{"backspace end", at("abcd", 4), "backspace", "abc", 3},
		{"backspace empty", New(""), "backspace", "", 0},
		{"ctrl+h middle", at("abcd", 2), "ctrl+h", "acd", 1},
		{"ctrl+h start", at("abcd", 0), "ctrl+h", "abcd", 0},
		{"delete middle", at("abcd", 2), "delete", "abd", 2},
		{"delete start", at("abcd", 0), "delete", "bcd", 0},
		{"delete end", at("abcd", 4), "delete", "abcd", 4},
		{"delete empty", New(""), "delete", "", 0},
		{"ctrl+d middle", at("abcd", 2), "ctrl+d", "abd", 2},
		{"ctrl+d end", at("abcd", 4), "ctrl+d", "abcd", 4},
		// ctrl+u / ctrl+k
		{"ctrl+u middle", at("abcd", 2), "ctrl+u", "cd", 0},
		{"ctrl+u start", at("abcd", 0), "ctrl+u", "abcd", 0},
		{"ctrl+u end", at("abcd", 4), "ctrl+u", "", 0},
		{"ctrl+u empty", New(""), "ctrl+u", "", 0},
		{"ctrl+k middle", at("abcd", 2), "ctrl+k", "ab", 2},
		{"ctrl+k start", at("abcd", 0), "ctrl+k", "", 0},
		{"ctrl+k end", at("abcd", 4), "ctrl+k", "abcd", 4},
		{"ctrl+k empty", New(""), "ctrl+k", "", 0},
		// typing
		{"type middle", at("abcd", 2), "X", "abXcd", 3},
		{"type start", at("abcd", 0), "X", "Xabcd", 1},
		{"type end", at("abcd", 4), "X", "abcdX", 5},
		{"type empty", New(""), "X", "X", 1},
		{"space middle", at("abcd", 2), "space", "ab cd", 3},

		// words over a path: letters and digits, so / - . _ are boundaries
		{"alt+b path end", New("/home/me/proj-a"), "alt+b", "/home/me/proj-a", 14},
		{"alt+b path after dash", at("/home/me/proj-a", 14), "alt+b", "/home/me/proj-a", 9},
		{"alt+b path mid word", at("/home/me/proj-a", 11), "alt+b", "/home/me/proj-a", 9},
		{"alt+b path to me", at("/home/me/proj-a", 9), "alt+b", "/home/me/proj-a", 6},
		{"alt+b path to home", at("/home/me/proj-a", 6), "alt+b", "/home/me/proj-a", 1},
		{"alt+b path to start", at("/home/me/proj-a", 1), "alt+b", "/home/me/proj-a", 0},
		{"alt+b path start", at("/home/me/proj-a", 0), "alt+b", "/home/me/proj-a", 0},
		{"ctrl+left path end", New("/home/me/proj-a"), "ctrl+left", "/home/me/proj-a", 14},
		{"ctrl+left path to proj", at("/home/me/proj-a", 14), "ctrl+left", "/home/me/proj-a", 9},
		{"alt+f path start", at("/home/me/proj-a", 0), "alt+f", "/home/me/proj-a", 5},
		{"alt+f path home", at("/home/me/proj-a", 5), "alt+f", "/home/me/proj-a", 8},
		{"alt+f path me", at("/home/me/proj-a", 8), "alt+f", "/home/me/proj-a", 13},
		{"alt+f path proj", at("/home/me/proj-a", 13), "alt+f", "/home/me/proj-a", 15},
		{"alt+f path end", at("/home/me/proj-a", 15), "alt+f", "/home/me/proj-a", 15},
		{"ctrl+right path start", at("/home/me/proj-a", 0), "ctrl+right", "/home/me/proj-a", 5},
		{"ctrl+right path mid word", at("/home/me/proj-a", 3), "ctrl+right", "/home/me/proj-a", 5},
		{"alt+backspace path end", New("/home/me/proj-a"), "alt+backspace", "/home/me/proj-", 14},
		{"alt+backspace path after dash", at("/home/me/proj-a", 14), "alt+backspace", "/home/me/a", 9},
		{"alt+backspace path mid word", at("/home/me/proj-a", 11), "alt+backspace", "/home/me/oj-a", 9},
		{"alt+backspace path start", at("/home/me/proj-a", 0), "alt+backspace", "/home/me/proj-a", 0},
		{"ctrl+w path end", New("/home/me/proj-a"), "ctrl+w", "", 0},
		{"ctrl+w path mid", at("/home/me/proj-a", 9), "ctrl+w", "proj-a", 0},

		// words over text with a space and a dash
		{"alt+b foo end", New("foo bar-baz"), "alt+b", "foo bar-baz", 8},
		{"alt+b foo baz start", at("foo bar-baz", 8), "alt+b", "foo bar-baz", 4},
		{"alt+b foo bar start", at("foo bar-baz", 4), "alt+b", "foo bar-baz", 0},
		{"alt+f foo start", at("foo bar-baz", 0), "alt+f", "foo bar-baz", 3},
		{"alt+f foo to bar", at("foo bar-baz", 3), "alt+f", "foo bar-baz", 7},
		{"alt+f foo to baz", at("foo bar-baz", 7), "alt+f", "foo bar-baz", 11},
		{"ctrl+left foo", New("foo bar-baz"), "ctrl+left", "foo bar-baz", 8},
		{"ctrl+right foo", at("foo bar-baz", 3), "ctrl+right", "foo bar-baz", 7},
		{"alt+backspace foo end", New("foo bar-baz"), "alt+backspace", "foo bar-", 8},
		{"alt+backspace foo dash", at("foo bar-baz", 8), "alt+backspace", "foo baz", 4},
		{"ctrl+w foo end", New("foo bar-baz"), "ctrl+w", "foo ", 4},
		{"ctrl+w foo after space", at("foo bar-baz", 4), "ctrl+w", "bar-baz", 0},
		{"ctrl+w foo trailing space", at("foo bar-baz", 3), "ctrl+w", " bar-baz", 0},
		{"alt+b digits", New("v2 build9"), "alt+b", "v2 build9", 3},
		{"alt+f empty", New(""), "alt+f", "", 0},
		{"alt+b empty", New(""), "alt+b", "", 0},
		{"ctrl+w empty", New(""), "ctrl+w", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, handled := c.from.Update(key(c.key))
			if !handled {
				t.Fatalf("%s not handled", c.key)
			}
			if got.Value() != c.value || got.Caret() != c.caret {
				t.Fatalf("got %q caret %d, want %q caret %d", got.Value(), got.Caret(), c.value, c.caret)
			}
		})
	}
}

func TestKeysTheEditorDoesNotOwn(t *testing.T) {
	for _, k := range []string{"up", "enter", "tab", "alt+w"} {
		e := NewOffered("abc")
		got, handled := e.Update(key(k))
		if handled {
			t.Errorf("%s handled, want left to the caller", k)
		}
		if got != e || !got.Offered() {
			t.Errorf("%s changed the editor or accepted the offer", k)
		}
	}
}

func TestInsertAndPasteInTheMiddle(t *testing.T) {
	e, _ := at("abcd", 2).Update(key("X"))
	if e.Value() != "abXcd" || e.Caret() != 3 {
		t.Fatalf("typed: %q %d", e.Value(), e.Caret())
	}
	e, handled := at("abcd", 2).Update(paste("123"))
	if !handled || e.Value() != "ab123cd" || e.Caret() != 5 {
		t.Fatalf("pasted: %q %d", e.Value(), e.Caret())
	}
}

func TestPasteDropsControlCharacters(t *testing.T) {
	e, _ := at("abcd", 2).Update(paste("x\ny\rz\tw\n"))
	if e.Value() != "abxyzwcd" || e.Caret() != 6 {
		t.Fatalf("got %q caret %d", e.Value(), e.Caret())
	}
	for _, c := range []string{"\n", "\r", "\t"} {
		if got, _ := New("ab").Update(paste("p" + c + "q")); got.Value() != "abpq" {
			t.Errorf("paste with %q: %q", c, got.Value())
		}
	}
}

func TestGraphemeClusters(t *testing.T) {
	// e + combining acute, a ZWJ family emoji, and a wide CJK character.
	const combining = "é"
	const zwj = "\U0001F468\u200d\U0001F469\u200d\U0001F467"
	const cjk = "漢"
	for _, c := range []struct{ name, char string }{
		{"combining", combining}, {"zwj emoji", zwj}, {"wide cjk", cjk},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := "a" + c.char + "b"
			e := New(text)
			e, _ = e.Update(key("left")) // before b
			e, _ = e.Update(key("left")) // before the cluster
			if e.Caret() != 1 {
				t.Fatalf("left stepped to %d, want 1", e.Caret())
			}
			e, _ = e.Update(key("right"))
			if e.Caret() != 1+len(c.char) {
				t.Fatalf("right stepped to %d, want %d", e.Caret(), 1+len(c.char))
			}
			bs, _ := e.Update(key("backspace"))
			if bs.Value() != "ab" || bs.Caret() != 1 {
				t.Fatalf("backspace: %q %d", bs.Value(), bs.Caret())
			}
			e, _ = e.Update(key("left"))
			del, _ := e.Update(key("delete"))
			if del.Value() != "ab" || del.Caret() != 1 {
				t.Fatalf("delete: %q %d", del.Value(), del.Caret())
			}
		})
	}
}

func TestOfferedRule(t *testing.T) {
	const offered = "/home/me"
	t.Run("printable replaces", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(key("x"))
		if e.Value() != "x" || e.Caret() != 1 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
	t.Run("space replaces", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(key("space"))
		if e.Value() != " " || e.Offered() {
			t.Fatalf("got %q offered=%v", e.Value(), e.Offered())
		}
	})
	t.Run("paste replaces", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(paste("/tmp/x\n"))
		if e.Value() != "/tmp/x" || e.Caret() != 6 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
	t.Run("left accepts and moves", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(key("left"))
		if e.Value() != offered || e.Caret() != len(offered)-1 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
	t.Run("backspace accepts and deletes one character", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(key("backspace"))
		if e.Value() != "/home/m" || e.Caret() != 7 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
	t.Run("every caret and editing key accepts", func(t *testing.T) {
		for _, k := range []string{"right", "ctrl+b", "ctrl+f", "home", "end", "ctrl+a", "ctrl+e",
			"alt+b", "alt+f", "ctrl+left", "ctrl+right", "delete", "ctrl+d", "ctrl+h", "ctrl+w",
			"alt+backspace", "ctrl+u", "ctrl+k"} {
			if e, handled := NewOffered(offered).Update(key(k)); !handled || e.Offered() {
				t.Errorf("%s: handled=%v offered=%v", k, handled, e.Offered())
			}
		}
	})
	t.Run("a typed value is not offered", func(t *testing.T) {
		if New(offered).Offered() {
			t.Fatal("New is offered")
		}
		if NewOffered("").Offered() {
			t.Fatal("empty text is offered")
		}
	})
	t.Run("a paste of only control characters still replaces the offer", func(t *testing.T) {
		for _, p := range []string{"\n", "\r\n\t", ""} {
			e, handled := NewOffered(offered).Update(paste(p))
			if !handled || e.Value() != "" || e.Caret() != 0 || e.Offered() {
				t.Errorf("paste %q: handled=%v got %q %d offered=%v", p, handled, e.Value(), e.Caret(), e.Offered())
			}
		}
	})
	t.Run("a paste of only control characters into an accepted value changes nothing", func(t *testing.T) {
		e, handled := at("abcd", 2).Update(paste("\r\n"))
		if !handled || e.Value() != "abcd" || e.Caret() != 2 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
	t.Run("a paste with a tab replaces the whole offer, tab dropped", func(t *testing.T) {
		e, _ := NewOffered(offered).Update(paste("a\tb"))
		if e.Value() != "ab" || e.Caret() != 2 || e.Offered() {
			t.Fatalf("got %q %d offered=%v", e.Value(), e.Caret(), e.Offered())
		}
	})
}

func TestConstructorsDropControlCharacters(t *testing.T) {
	const raw = "a\x1b[2Jb\x07c\r\nd\te\u009bf\x7f"
	const want = "a[2Jbcdef"
	for name, e := range map[string]Editor{"New": New(raw), "NewOffered": NewOffered(raw)} {
		if e.Value() != want {
			t.Errorf("%s(%q).Value() = %q, want %q", name, raw, e.Value(), want)
		}
		if e.Caret() != len(want) {
			t.Errorf("%s caret = %d, want the end of the stripped text (%d)", name, e.Caret(), len(want))
		}
	}
	if !NewOffered(raw).Offered() {
		t.Error("NewOffered of a non-empty value must be offered")
	}
	if NewOffered("\x1b\x07").Offered() {
		t.Error("NewOffered of only control characters holds nothing, so it offers nothing")
	}
}
