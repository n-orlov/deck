package features

import "testing"

// TestUnarchiveRepaintedWaitsForBothTheRowAndTheFooter pins the wait
// clientUnarchivesSelectedSession adds after U: the database alone is not
// enough, deck's own repaint of the row (badge gone) and of the footer (no U
// offer) must both have landed before the next step may send a key.
func TestUnarchiveRepaintedWaitsForBothTheRowAndTheFooter(t *testing.T) {
	const archivedRow = "> o trip-target stopped [arch... |\n"
	const plainRow = "> o trip-target stopped          |\n"
	const uFooter = "resumable    up/down - n new - r resume - dd delete - U unarchive - , settings - ...\n"
	const aFooter = "resumable    up/down - n new - r resume - dd delete - A archive - , settings - ...\n"
	cases := []struct {
		name  string
		frame string
		want  bool
	}{
		{"row still badged, footer still offers U", archivedRow + uFooter, false},
		{"row repainted, footer not yet", plainRow + uFooter, false},
		{"footer repainted, row not yet", archivedRow + aFooter, false},
		{"unicode badge still on the row", "> o trip-target stopped ▣ |\n" + aFooter, false},
		{"both repainted", plainRow + aFooter, true},
	}
	for _, c := range cases {
		if got := unarchiveRepainted(c.frame); got != c.want {
			t.Errorf("%s: unarchiveRepainted = %v, want %v (selected line %q)", c.name, got, c.want, selectedSidebarLine(c.frame))
		}
	}
}
