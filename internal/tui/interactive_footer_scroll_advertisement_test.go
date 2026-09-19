// interactive_footer_scroll_advertisement_test.go is R134's own proof
// (PRD phase4b, GH #30): interactiveFooterLine names the Shift+PgUp/PgDn
// scroll keys that interactive mode already binds (updateInteractive's
// wheel and key handling), and it must never let that new segment push
// the rendered line past the frame's own width -- degrading exactly the
// way footerLegendWithin degrades the list-mode legend (whole segments
// drop from the least essential end) rather than overflowing, and never
// dropping Ctrl+Q, the one way out of interactive mode.
//
// These tests call interactiveFooterLine directly on a Model with no live
// tmux grid (m.interactiveGrid == nil): interactiveAtTopOfScrollback
// already treats a nil grid as "not at the top" (tui.go), so every offset
// here renders the ordinary "Scrolled back N lines" wording rather than
// the top-of-scrollback one -- exactly what this file needs to hold the
// cue's own text, and hence its own width, fixed while only the frame
// width varies across the table below.
package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// footerAtWidth builds an interactive Model scrolled back by offset lines
// and renders interactiveFooterLine at exactly width columns.
func footerAtWidth(width, offset int) string {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = width, 30
	m.interactive = true
	m.setInteractiveScrollOffset(offset)
	return m.interactiveFooterLine()
}

// TestInteractiveFooterNamesTheScrollKeysWhenRoomAllows is R134's naming
// claim: once scrolled back and given enough room, the footer's own text
// names Shift+PgUp/PgDn -- the key mouse_interactive_retarget_test.go and
// updateInteractive already bind for exactly this gesture.
func TestInteractiveFooterNamesTheScrollKeysWhenRoomAllows(t *testing.T) {
	footer := footerAtWidth(160, 5)
	if !strings.Contains(footer, "Shift+PgUp/PgDn") {
		t.Fatalf("footer = %q, want it to name Shift+PgUp/PgDn while scrolled back with room to spare", footer)
	}
	if !strings.Contains(footer, "Ctrl+Q") {
		t.Fatalf("footer = %q, want Ctrl+Q named alongside the scroll-key advertisement", footer)
	}
}

// TestInteractiveFooterAtTheLiveBottomNeverNamesTheScrollKeys is R134's own
// half of the PRD's sanctioned simplification: the scroll-key
// advertisement is shown only while scrolled back, so at the live bottom
// (offset 0) the footer must never mention it, at any width -- this is
// also the pre-existing guarantee interactive_footer_scroll_cue_test.go's
// "must not mention scrolling" assertion already pins for the cue itself.
func TestInteractiveFooterAtTheLiveBottomNeverNamesTheScrollKeys(t *testing.T) {
	for _, width := range []int{80, 120, 200} {
		footer := footerAtWidth(width, 0)
		if strings.Contains(strings.ToLower(footer), "scroll") {
			t.Fatalf("width=%d: footer = %q, must not mention scrolling at the live bottom", width, footer)
		}
		if !strings.Contains(footer, "Ctrl+Q") {
			t.Fatalf("width=%d: footer = %q, want Ctrl+Q always present", width, footer)
		}
	}
}

// TestInteractiveFooterDegradesWithoutOverflowingTheFrame is R134's own
// success criterion: at the golden 80-column width (and at every other
// width this table drives, each one forcing one more degradation step
// than the last) the rendered footer never exceeds the frame, and
// Ctrl+Q -- the one way out of interactive mode -- survives every single
// one of those steps, including the narrowest.
func TestInteractiveFooterDegradesWithoutOverflowingTheFrame(t *testing.T) {
	const offset = 5 // fixes the cue's own text (and width) across the table
	cases := []struct {
		width           int
		name            string
		wantContains    []string
		wantNotContains []string
	}{
		{200, "full: every optional segment fits", []string{"Scrolled back 5 lines", "Shift+PgUp/PgDn", "keystrokes forward to the live pane", "Ctrl+Q"}, nil},
		{100, "the forward-note reminder is dropped first", []string{"Scrolled back 5 lines", "Shift+PgUp/PgDn", "Ctrl+Q"}, []string{"keystrokes forward to the live pane"}},
		// 80 is the golden width R134 names explicitly: the cue and
		// scroll-key advertisement no longer both fit alongside the
		// mandatory Ctrl+Q segment, so the scroll-key advertisement --
		// lower priority than the cue's own "not live" safety
		// information -- is the next to drop.
		{80, "golden 80 columns: the scroll-key advertisement is dropped next", []string{"Scrolled back 5 lines", "Ctrl+Q"}, []string{"Shift+PgUp/PgDn"}},
		{60, "narrower still: even the cue itself is dropped", []string{"Ctrl+Q", "leave interactive mode"}, []string{"Scrolled back", "Shift+PgUp/PgDn"}},
		{20, "narrower than Ctrl+Q's own hint: only the bare key survives", []string{"Ctrl+Q"}, []string{"leave interactive mode", "Scrolled back", "Shift+PgUp/PgDn"}},
	}

	for _, tc := range cases {
		footer := footerAtWidth(tc.width, offset)
		if got := stringWidth(footer); got > tc.width {
			t.Fatalf("%s (width=%d): footer %q has visible width %d, want <= %d", tc.name, tc.width, footer, got, tc.width)
		}
		if !strings.Contains(footer, "Ctrl+Q") {
			t.Fatalf("%s (width=%d): footer = %q, Ctrl+Q must survive every degradation step", tc.name, tc.width, footer)
		}
		for _, want := range tc.wantContains {
			if !strings.Contains(footer, want) {
				t.Fatalf("%s (width=%d): footer = %q, want it to contain %q", tc.name, tc.width, footer, want)
			}
		}
		for _, notWant := range tc.wantNotContains {
			if strings.Contains(footer, notWant) {
				t.Fatalf("%s (width=%d): footer = %q, want it to NOT contain %q (this width forces that segment to drop)", tc.name, tc.width, footer, notWant)
			}
		}
	}
}

// TestInteractiveFooterScrollKeyWordingAgreesWithHelpAndMouseTables is
// R134's own agreement claim: helpText's Keys section and its Mouse
// table already document Shift+PgUp/PgDn (both pinned in
// cmd/deck/main_test.go and internal/tui/tui_test.go), and this task must
// not let the footer disagree with that existing wording once it starts
// naming the same keys itself.
func TestInteractiveFooterScrollKeyWordingAgreesWithHelpAndMouseTables(t *testing.T) {
	const wantSubstring = "Shift+PgUp/PgDn"

	footer := footerAtWidth(160, 5)
	if !strings.Contains(footer, wantSubstring) {
		t.Fatalf("footer = %q, want it to contain %q", footer, wantSubstring)
	}

	help := helpText(false)
	if !strings.Contains(help, wantSubstring) {
		t.Fatalf("helpText(false) does not contain %q -- it should already document the interactive scroll keys (the Keys section's ↵ entry) and the Mouse table's own wheel-over-the-preview row", wantSubstring)
	}
	if strings.Count(help, wantSubstring) < 2 {
		t.Fatalf("helpText(false) contains %q only %d time(s), want at least 2 (once in the Keys section's ↵ entry, once in the Mouse table's wheel-over-the-preview row) so the mouse table and the Keys section, and now the footer, all agree on the same wording", wantSubstring, strings.Count(help, wantSubstring))
	}
}
