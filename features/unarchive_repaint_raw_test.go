package features

import "testing"

// TestUnarchiveRepaintedReadsTheSidebarCellRaw pins the discriminating power of the
// wait clientUnarchivesSelectedSession ends on (filter.feature:97): a frame
// that still draws the selected row with its archived badge, or still offers
// to unarchive it, is NOT a repainted frame, and one that drew the row
// without the badge and without the offer is. The first row is the frame
// deck paints before it processes `U`; reading the row through
// selectedSidebarLine (which strips badge runs) used to call it repainted.
func TestUnarchiveRepaintedReadsTheSidebarCellRaw(t *testing.T) {
	frame := func(row, footer string) string {
		return "+ deck - sessions -------+----------+\n" +
			"| " + row + " | preview  |\n" +
			"+------------------------+----------+\n" + footer + "\n"
	}
	cases := []struct {
		name string
		got  string
		want bool
	}{
		{"badge truncated, archived note", frame("> # trip-target stopped [arch...", "Killed and archived — press u to unarchive (agent stays stopped)"), false},
		{"badge in full", frame("> # trip-target stopped [archived]", "stopped - re..."), false},
		{"unicode glyph badge", frame("> # trip-target stopped ▣", "stopped - re..."), false},
		{"legend still offers U", frame("> # trip-target stopped", "r resume  U unarchive"), false},
		{"no selected row drawn yet", frame("  # trip-target stopped", "stopped - re..."), false},
		{"row repainted without the badge", frame("> # trip-target stopped", "stopped - re...    up/down - ..."), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unarchiveRepainted(tc.got); got != tc.want {
				t.Errorf("unarchiveRepainted = %v, want %v for:\n%s", got, tc.want, tc.got)
			}
		})
	}
}
