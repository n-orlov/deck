package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/theme"
)

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

// cellReverseStates walks a rendered row and reports, for every printable
// rune, whether SGR 7 is in effect on it -- what a terminal would paint
// reversed, independent of how the sequences were spelled.
func cellReverseStates(row string) (cells string, reversed []bool) {
	var fg, bg sgrColor
	reverse := false
	var sb strings.Builder
	for i := 0; i < len(row); {
		if row[i] == 0x1b {
			n := ansiEscapeLen(row, i)
			if n >= 3 && row[i+1] == '[' && row[i+n-1] == 'm' {
				applySGR(row[i+2:i+n-1], &fg, &bg, &reverse)
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(row[i:])
		sb.WriteRune(r)
		reversed = append(reversed, reverse)
		i += size
	}
	return sb.String(), reversed
}

func reverseMask(cells string, reversed []bool) string {
	var sb strings.Builder
	for i := range reversed {
		if reversed[i] {
			sb.WriteByte('R')
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

// R182 / SPEC §11.9: a cell the program painted in reverse video is shown
// as painted under every preview_paint mode, and drawing the cursor must
// not disturb the reverse state of any cell around it -- in particular a
// cursor INSIDE a program-painted reverse run must not close that run.
func TestInteractiveCursorKeepsProgramReverseRunsUnderEveryPaintMode(t *testing.T) {
	cases := []struct {
		name string
		row  string
		col  int
	}{
		{"cursor mid-run", "ab\x1b[7mCDE\x1b[27mfg", 3},
		{"cursor on first cell of run", "ab\x1b[7mCDE\x1b[27mfg", 2},
		{"cursor on last cell of run", "ab\x1b[7mCDE\x1b[27mfg", 4},
		{"run reaches end of row", "ab\x1b[7mCDEfg", 3},
		{"run closed by full reset", "ab\x1b[7mCDE\x1b[0mfg", 4},
		{"reverse combined with colour", "ab\x1b[31;7mCDE\x1b[0mfg", 3},
		{"reverse set in a multi-parameter sequence", "ab\x1b[1;7mCDE\x1b[22;27mfg", 3},
		{"cursor outside run, run before it", "\x1b[7mab\x1b[27mcdfg", 3},
		{"cursor beyond row text after a run", "\x1b[7mab\x1b[27mcd", 5},
		{"two runs, cursor in the second", "\x1b[7ma\x1b[27mb\x1b[7mcd\x1b[27me", 3},
	}
	for _, mode := range []string{"fit", "nofit", "bg", "off"} {
		for _, th := range theme.Builtins() {
			m := paintModelMode(t, th.Name, mode)
			m.interactive = true
			for _, tc := range cases {
				cells, base := cellReverseStates(tc.row)
				drawn := drawInteractiveCursor([]string{tc.row}, 0, 0, tc.col, true)[0]
				painted := m.previewContentLine(40, drawn, false)
				pcells, got := cellReverseStates(painted)
				// Offset of the row's own text inside the painted line: the
				// border/pad cells deck adds are never reversed.
				off := strings.Index(pcells, cells)
				if off >= 0 {
					off = utf8.RuneCountInString(pcells[:off]) // got is indexed by cell, not byte
				}
				if off < 0 {
					t.Fatalf("%s/%s/%s: text %q missing from %q", mode, th.Name, tc.name, cells, pcells)
				}
				// Every program-painted reverse cell stays reverse; every
				// other cell stays non-reverse, except the cursor cell.
				for i := range base {
					if off+i >= len(got) {
						t.Fatalf("%s/%s/%s: painted row lost cell %d", mode, th.Name, tc.name, i)
					}
					want := base[i] || i == tc.col
					if got[off+i] != want {
						t.Errorf("%s/%s/%s: cell %d (%q) reverse=%v, want %v; mask %s",
							mode, th.Name, tc.name, i, cells[i:i+1], got[off+i], want,
							reverseMask(pcells, got))
					}
				}
			}
		}
	}
}
