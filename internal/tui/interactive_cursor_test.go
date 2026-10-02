package tui

import "testing"

func TestDrawInteractiveCursorReversesExactlyTheCursorCell(t *testing.T) {
	rows := []string{"abc   ", "      "}
	got := drawInteractiveCursor(rows, 0, 0, 1, true)
	if want := "a\x1b[7mb\x1b[27mc   "; got[0] != want {
		t.Fatalf("row 0 = %q, want %q", got[0], want)
	}
	if got[1] != rows[1] {
		t.Fatalf("row 1 changed: %q", got[1])
	}
	if rows[0] != "abc   " {
		t.Fatalf("the caller's rows were mutated: %q", rows[0])
	}
}

func TestDrawInteractiveCursorDrawsNothingWhenItMustNot(t *testing.T) {
	rows := []string{"abc", "def"}
	for name, got := range map[string][]string{
		"hidden cursor":         drawInteractiveCursor(rows, 0, 0, 1, false),
		"scrolled back":         drawInteractiveCursor(rows, 3, 0, 1, true),
		"cursor row above view": drawInteractiveCursor(rows, 0, -1, 1, true),
		"cursor row below view": drawInteractiveCursor(rows, 0, 2, 1, true),
		"negative column":       drawInteractiveCursor(rows, 0, 0, -1, true),
	} {
		if got[0] != "abc" || got[1] != "def" {
			t.Errorf("%s: rows changed to %q", name, got)
		}
	}
}

func TestReverseCellSGRWalksPastEscapesAndWideCells(t *testing.T) {
	// The escape costs no columns; the wide rune costs two, so column 3 is
	// the 'z' after it.
	line := "\x1b[31mx\x1b[0m世z"
	if got, want := reverseCellSGR(line, 3), "\x1b[31mx\x1b[0m世\x1b[7mz\x1b[27m"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// A cursor on the wide cell's trailing half still marks the glyph.
	if got, want := reverseCellSGR(line, 2), "\x1b[31mx\x1b[0m\x1b[7m世\x1b[27mz"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReverseCellSGRPadsAShortRow(t *testing.T) {
	if got, want := reverseCellSGR("ab", 4), "ab  \x1b[7m \x1b[27m"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
