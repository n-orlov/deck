package tui

import "testing"

// TestAnsiEscapeLenMeasuresEachEscapeForm pins the contract panel.go's
// ansiEscapeLen documents: the byte length of the escape sequence starting
// at s[i] (CSI, OSC with either terminator, or ESC plus one rune), 0 when
// no escape starts there, and always at least one byte so a scanner
// advancing by it cannot loop forever.
func TestAnsiEscapeLenMeasuresEachEscapeForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    string
		i    int
		want int
	}{
		{"plain text is not an escape", "abc", 0, 0},
		{"index past the end", "abc", 3, 0},
		{"index beyond the end", "abc", 10, 0},
		{"empty string", "", 0, 0},
		{"a lone trailing ESC is one byte", "ab\x1b", 2, 1},
		{"SGR colour", "\x1b[31mred", 0, 5},
		{"SGR with several parameters", "\x1b[38;2;10;20;30mx", 0, 16},
		{"SGR reset", "\x1b[0m", 0, 4},
		{"CSI with no parameters", "\x1b[mx", 0, 3},
		{"CSI cursor move ends on its final byte", "\x1b[12;40Hrest", 0, 8},
		{"CSI missing its final byte consumes to the end", "\x1b[31", 0, 4},
		{"bare CSI introducer at the end", "\x1b[", 0, 2},
		{"OSC terminated by BEL", "\x1b]0;title\x07tail", 0, 10},
		{"OSC terminated by ST", "\x1b]8;;http://x\x1b\\tail", 0, 15},
		{"OSC with an ESC that is not ST keeps scanning", "\x1b]0;a\x1bb\x07z", 0, 8},
		{"unterminated OSC consumes to the end", "\x1b]0;never ends", 0, 14},
		{"two-byte escape", "\x1bcrest", 0, 2},
		{"ESC plus a multi-byte rune advances a whole rune", "\x1bébc", 0, 3},
		{"escape found at a non-zero offset", "ab\x1b[1mcd", 2, 4},
		{"offset on a non-ESC byte inside a sequence", "\x1b[31m", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ansiEscapeLen(tc.s, tc.i); got != tc.want {
				t.Fatalf("ansiEscapeLen(%q, %d) = %d, want %d", tc.s, tc.i, got, tc.want)
			}
		})
	}
}

func TestAnsiEscapeLenNeverStallsAScanner(t *testing.T) {
	for _, s := range []string{"\x1b", "\x1b[", "\x1b]", "\x1b\x1b", "\x1b[\x1b", "\x1b]\x1b", "\x1b\xff"} {
		for i := 0; i < len(s); i++ {
			if s[i] != 0x1b {
				continue
			}
			if got := ansiEscapeLen(s, i); got < 1 || i+got > len(s) {
				t.Errorf("ansiEscapeLen(%q, %d) = %d: must advance 1..%d bytes", s, i, got, len(s)-i)
			}
		}
	}
}
