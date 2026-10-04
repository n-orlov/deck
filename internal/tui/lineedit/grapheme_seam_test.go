package lineedit

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

// onBoundary reports whether off is a grapheme boundary of text.
func onBoundary(text string, off int) bool {
	if off == 0 || off == len(text) {
		return true
	}
	for pos := 0; pos < len(text); {
		if pos == off {
			return true
		}
		cl, _, _, _ := uniseg.FirstGraphemeClusterInString(text[pos:], -1)
		pos += len(cl)
		if pos > off {
			return false
		}
	}
	return false
}

func mustBoundary(t *testing.T, e Editor, what string) {
	t.Helper()
	if !onBoundary(e.Value(), e.Caret()) {
		t.Fatalf("%s: caret %d is inside a cluster of %q", what, e.Caret(), e.Value())
	}
}

const (
	woman  = "\U0001F469"
	laptop = "\U0001F4BB"
	zwj    = "\u200d"
	flagA  = "\U0001F1E6"
	flagB  = "\U0001F1E7"
	acute  = "́"
)

func TestSeamJoinsKeepCaretOnABoundaryAndRenderACaret(t *testing.T) {
	for _, c := range []struct {
		name       string
		start      Editor // caret placed at the seam
		mutate     func(Editor) Editor
		wantValue  string
		wantBefore string // text before the caret afterwards
	}{
		{"insert ZWJ between woman and laptop", seam(woman+laptop, len(woman)),
			func(e Editor) Editor { return e.Insert(zwj) }, woman + zwj + laptop, woman + zwj + laptop},
		{"paste ZWJ between woman and laptop", seam(woman+laptop, len(woman)),
			func(e Editor) Editor { return e.Paste(zwj) }, woman + zwj + laptop, woman + zwj + laptop},
		{"insert e before a leading combining acute", seam(acute, 0),
			func(e Editor) Editor { return e.Insert("e") }, "e" + acute, "e" + acute},
		{"insert a regional indicator before a flag", seam(flagA+flagB, 0),
			func(e Editor) Editor { return e.Insert(flagA) }, flagA + flagA + flagB, flagA + flagA},
		{"paste a combining mark after a letter in the middle", seam("ab", 1),
			func(e Editor) Editor { return e.Paste(acute) }, "a" + acute + "b", "a" + acute},
		{"delete x between two regional indicators", seam(flagA+"x"+flagB, len(flagA)+1),
			func(e Editor) Editor { r, _ := e.Update(key("backspace")); return r }, flagA + flagB, ""},
		{"forward delete x between a woman, ZWJ and laptop", seam(woman+zwj+"x"+laptop, len(woman+zwj)),
			func(e Editor) Editor { r, _ := e.Update(key("delete")); return r }, woman + zwj + laptop, ""},
		{"ctrl+k never leaves a caret mid-cluster", seam("a"+flagA+flagB, 1),
			func(e Editor) Editor { r, _ := e.Update(key("ctrl+k")); return r }, "a", "a"},
		{"backspace joins a letter and a combining mark across a space", seam("e x"+acute, 2),
			func(e Editor) Editor { r, _ := e.Update(key("backspace")); return r }, "ex" + acute, "e"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := c.mutate(c.start)
			if e.Value() != c.wantValue {
				t.Fatalf("value %q, want %q", e.Value(), c.wantValue)
			}
			mustBoundary(t, e, "after the mutation")
			if c.wantBefore != "" {
				if before, _ := e.Split(); before != c.wantBefore {
					t.Fatalf("before the caret %q, want %q", before, c.wantBefore)
				}
			}
			if out := e.View(20, Style{}); strings.Count(out, "\x1b[7m") != 1 {
				t.Fatalf("focused render has no single reverse-video caret: %q", out)
			}
		})
	}
}

// seam builds an unoffered editor over text with its caret at off.
func seam(text string, off int) Editor {
	e := New(text)
	e.caret = off
	return e
}

func TestMovementAndDeletionAfterAJoinUseTheWholeCluster(t *testing.T) {
	// ZWJ typed between woman and laptop: one complete grapheme remains.
	e := seam(woman+laptop, len(woman)).Insert(zwj)
	left, _ := e.Update(key("left"))
	if left.Caret() != 0 {
		t.Fatalf("left over the joined emoji stepped to %d, want 0", left.Caret())
	}
	back, _ := e.Update(key("backspace"))
	if back.Value() != "" || back.Caret() != 0 {
		t.Fatalf("backspace left %q at %d, want the whole grapheme gone", back.Value(), back.Caret())
	}
	home, _ := e.Update(key("home"))
	right, _ := home.Update(key("right"))
	if right.Caret() != len(e.Value()) {
		t.Fatalf("right stepped to %d, want %d", right.Caret(), len(e.Value()))
	}
	del, _ := home.Update(key("delete"))
	if del.Value() != "" {
		t.Fatalf("delete left %q", del.Value())
	}

	// A flag formed by deleting the x between its indicators.
	f := seam(flagA+"x"+flagB, len(flagA)+1)
	f, _ = f.Update(key("backspace"))
	if f.Caret() != 0 {
		t.Fatalf("caret %d after the join, want it before the flag", f.Caret())
	}
	b, _ := f.Update(key("delete"))
	if b.Value() != "" {
		t.Fatalf("delete left %q, want the whole flag gone", b.Value())
	}
	f, _ = f.Update(key("home"))
	f, _ = f.Update(key("right"))
	if f.Caret() != len(flagA+flagB) {
		t.Fatalf("right over the flag stepped to %d", f.Caret())
	}

	// Typing a letter, then an accent, then stepping back deletes both.
	g := New("ab")
	g, _ = g.Update(key("left"))
	g = g.Insert(acute) // joins onto a
	if v, _ := g.Split(); v != "a"+acute {
		t.Fatalf("before caret %q", v)
	}
	g, _ = g.Update(key("backspace"))
	if g.Value() != "b" || g.Caret() != 0 {
		t.Fatalf("backspace left %q at %d, want the accented letter gone whole", g.Value(), g.Caret())
	}
}

func TestEveryMutationKeepsTheCaretOnABoundary(t *testing.T) {
	texts := []string{"", "a", woman + laptop, flagA + flagB, "e" + acute + "x", flagA + "x" + flagB + zwj + laptop}
	pieces := []string{"x", zwj, acute, flagA, woman, "e" + acute}
	keys := []string{"backspace", "delete", "ctrl+w", "ctrl+u", "ctrl+k", "alt+backspace"}
	for _, text := range texts {
		for off := 0; off <= len(text); off++ {
			if !onBoundary(text, off) {
				continue
			}
			for _, p := range pieces {
				mustBoundary(t, seam(text, off).Insert(p), "insert "+p)
				mustBoundary(t, seam(text, off).Paste(p), "paste "+p)
			}
			for _, k := range keys {
				e, _ := seam(text, off).Update(key(k))
				mustBoundary(t, e, k+" on "+text)
			}
		}
	}
}
