package tui

import (
	"bytes"
	"testing"

	"github.com/n-orlov/deck/internal/interactive"
)

// TestWheelRoutingTable pins the R183 routing rule over reporting mode x
// grid offset x Shift. The oracle is written out from the requirement:
// forward when 1000, 1002 or 1003 is on, the grid is at live and Shift is
// not held; 1005, 1006 and 1015 only choose an encoding and never switch
// reporting on by themselves.
func TestWheelRoutingTable(t *testing.T) {
	modes := []struct {
		name      string
		m         interactive.MouseMode
		reporting bool
	}{
		{"none", 0, false},
		{"1000", interactive.MouseNormal, true},
		{"1002", interactive.MouseButton, true},
		{"1003", interactive.MouseAny, true},
		{"1000+1006", interactive.MouseNormal | interactive.MouseSGR, true},
		{"1003+1005", interactive.MouseAny | interactive.MouseUTF8, true},
		{"1002+1015", interactive.MouseButton | interactive.MouseURXVT, true},
		{"1006 alone", interactive.MouseSGR, false},
		{"1005 alone", interactive.MouseUTF8, false},
		{"1015 alone", interactive.MouseURXVT, false},
	}
	for _, md := range modes {
		for _, offset := range []int{0, 1, 40} {
			for _, shift := range []bool{false, true} {
				want := md.reporting && offset == 0 && !shift
				if got := wheelForwardsToProgram(md.m, offset, shift); got != want {
					t.Errorf("modes %s offset %d shift %v: forward = %v, want %v", md.name, offset, shift, got, want)
				}
			}
		}
	}
}

// TestEncodeWheelReportBytes pins both encodings byte for byte. SGR is
// ESC [ < b ; col+1 ; row+1 M with b 64 (up) or 65 (down); X10 is ESC [ M
// followed by 32+b, 32+col+1 and 32+row+1, so the largest 1-based
// coordinate it can carry is 223 (byte 255) and one past it is dropped.
func TestEncodeWheelReportBytes(t *testing.T) {
	sgr := interactive.MouseNormal | interactive.MouseSGR
	x10 := interactive.MouseNormal
	esc := "\x1b"
	for _, tc := range []struct {
		name     string
		modes    interactive.MouseMode
		up       bool
		col, row int
		want     string // "" = dropped
	}{
		{"sgr up origin", sgr, true, 0, 0, esc + "[<64;1;1M"},
		{"sgr down origin", sgr, false, 0, 0, esc + "[<65;1;1M"},
		{"sgr up (9,4)", sgr, true, 9, 4, esc + "[<64;10;5M"},
		{"sgr down (79,23)", sgr, false, 79, 23, esc + "[<65;80;24M"},
		{"sgr far past 223 is fine", sgr, true, 223, 300, esc + "[<64;224;301M"},
		{"sgr with 1003", interactive.MouseAny | interactive.MouseSGR, false, 5, 6, esc + "[<65;6;7M"},
		{"sgr wins over 1005", sgr | interactive.MouseUTF8, true, 1, 1, esc + "[<64;2;2M"},
		{"x10 up origin", x10, true, 0, 0, esc + "[M" + string([]byte{96, 33, 33})},
		{"x10 down origin", x10, false, 0, 0, esc + "[M" + string([]byte{97, 33, 33})},
		{"x10 up (9,4)", x10, true, 9, 4, esc + "[M" + string([]byte{96, 42, 37})},
		{"x10 col 222 (1-based 223) last carried", x10, true, 222, 0, esc + "[M" + string([]byte{96, 255, 33})},
		{"x10 col 223 (1-based 224) dropped", x10, true, 223, 0, ""},
		{"x10 row 222 (1-based 223) last carried", x10, false, 0, 222, esc + "[M" + string([]byte{97, 33, 255})},
		{"x10 row 223 (1-based 224) dropped", x10, false, 0, 223, ""},
		{"x10 both 222", x10, true, 222, 222, esc + "[M" + string([]byte{96, 255, 255})},
		{"x10 col 500 dropped", x10, true, 500, 3, ""},
		{"1005 dropped at origin", interactive.MouseNormal | interactive.MouseUTF8, true, 0, 0, ""},
		{"1005 dropped at (10,10)", interactive.MouseAny | interactive.MouseUTF8, false, 10, 10, ""},
		{"1015 dropped at origin", interactive.MouseNormal | interactive.MouseURXVT, true, 0, 0, ""},
		{"1015 dropped at (10,10)", interactive.MouseButton | interactive.MouseURXVT, false, 10, 10, ""},
	} {
		got, ok := encodeWheelReport(tc.modes, tc.up, tc.col, tc.row)
		if tc.want == "" {
			if ok || len(got) != 0 {
				t.Errorf("%s: got %q (ok=%v), want it dropped", tc.name, got, ok)
			}
			continue
		}
		if !ok || !bytes.Equal(got, []byte(tc.want)) {
			t.Errorf("%s: got %q (ok=%v), want %q", tc.name, got, ok, tc.want)
		}
	}
}
