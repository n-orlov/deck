// r157_wheel_help_footer_test.go is task 021's own render-level proof for
// R157: the `?` help view and the footer must describe the interactive
// sidebar wheel exactly as SPEC's amended §11.8/§11.9 do -- the panel
// under the pointer decides what a wheel notch scrolls while interactive
// (over the preview, the grid's own scrollback; over the sidebar, the
// list itself, without leaving interactive mode or resizing anything),
// per the operator's phase 4e amendment (commit ad3398531a). Before task
// 021 the Keys section's `↵` entry claimed, unqualified, that "a wheel
// notch ... scrolls this bounded, deck-owned scrollback of the fitted
// view" -- true only for a wheel over the preview, and actively wrong for
// a wheel over the sidebar since R149 (GH #47) routed that one to
// scrollSidebar instead. This file pins the corrected wording so a later
// edit cannot silently reintroduce the same contradiction.
package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestHelpKeysEntryScopesTheInteractiveWheelByPanel proves the Keys
// section's `↵` bullet no longer makes an unqualified "a wheel notch"
// claim: it must say a wheel notch over the sidebar scrolls the list
// instead, without leaving interactive mode or resizing anything --
// matching SPEC §11.9's "the wheel over the preview scrolls it (over the
// sidebar the wheel scrolls the list, §11.8)".
func TestHelpKeysEntryScopesTheInteractiveWheelByPanel(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		enter := "\u21b5"
		if ascii {
			enter = "Enter"
		}
		row := renderedHelpBullet(t, ascii, enter+" enter interactive mode on the selected running session", "force-enter interactive mode")
		for _, want := range []string{
			"scrolls this bounded, deck-owned scrollback of the fitted view",
			"a wheel notch over the sidebar instead scrolls the list",
			"without leaving interactive mode or resizing anything",
		} {
			if !strings.Contains(row, want) {
				t.Errorf("ascii=%v: rendered \u21b5 bullet does not name %q (SPEC \u00a711.8/\u00a711.9: the panel under the pointer decides what the wheel scrolls):\n%s", ascii, want, row)
			}
		}
	}
}

// TestHelpMouseRowSidebarWheelStatesInteractiveContinuity proves the
// Mouse table's own "wheel over the sidebar" row states the same
// continuity: it behaves identically while interactive, never leaving
// interactive mode or resizing anything -- SPEC §11.8's own amended
// paragraph ("over the sidebar it scrolls the list exactly as it does in
// list mode ... without leaving interactive mode or resizing anything").
func TestHelpMouseRowSidebarWheelStatesInteractiveContinuity(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		row := renderedHelpBullet(t, ascii, "wheel over the sidebar", "wheel over an overlay")
		for _, want := range []string{
			"scroll the list without changing selection",
			"while interactive",
			"leaving interactive mode",
			"resizing anything",
		} {
			if !strings.Contains(row, want) {
				t.Errorf("ascii=%v: rendered \"wheel over the sidebar\" mouse row does not name %q (SPEC \u00a711.8: the panel under the pointer decides what the wheel scrolls, and the sidebar case never leaves interactive mode or resizes anything):\n%s", ascii, want, row)
			}
		}
	}
}

// TestFooterAdvertisesTheKeyTheSidebarWheelDuplicates proves the other
// half of R157's own success criterion: the footer, not only the help
// view, describes the capability. SPEC §11.8's rule is that every mouse
// binding duplicates a key, and the key remains the primary path -- for
// "wheel over the sidebar" that key is the unconditional `↑/↓` entry
// (footerLegend's own first row), present in the list-mode footer legend
// whether or not any row is selected (nil eligible predicate), so the
// duplicated key the sidebar wheel stands in for is never itself absent
// from the one line SPEC §11.3 says the footer must never contradict.
func TestFooterAdvertisesTheKeyTheSidebarWheelDuplicates(t *testing.T) {
	m := New(nil, config.Settings{Mouse: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	m.selected = rowCursor(0)

	footer := stripANSI(m.footerLine())
	if !strings.Contains(footer, "\u2191/\u2193") {
		t.Fatalf("list-mode footer %q does not advertise \u2191/\u2193, the key \"wheel over the sidebar\" duplicates (SPEC \u00a711.8's table)", footer)
	}

	// While interactive, the footer's own scroll-key advertisement
	// (Shift+PgUp/PgDn) is scoped to the interactive PREVIEW's bounded
	// scrollback -- it must never claim to describe the sidebar, which
	// the wheel also reaches while interactive but through a gesture the
	// footer's one-line budget does not carry a key for (arrows are
	// forwarded to the live pane instead, per SPEC \u00a711.9). Confirming the
	// interactive footer's own wording stays scoped to "the live pane"
	// and the preview's own scroll segment -- never to "sidebar" or
	// "list" -- keeps the two lines from disagreeing about which panel a
	// wheel notch moves while interactive.
	inter := m
	inter.interactive = true
	inter.setInteractiveScrollOffset(5)
	interFooter := stripANSI(inter.interactiveFooterLine())
	if strings.Contains(strings.ToLower(interFooter), "sidebar") || strings.Contains(strings.ToLower(interFooter), "list") {
		t.Fatalf("interactive footer %q wrongly names the sidebar/list -- its Shift+PgUp/PgDn segment must stay scoped to the interactive preview's own scrollback", interFooter)
	}
	if !strings.Contains(interFooter, "Shift+PgUp/PgDn") {
		t.Fatalf("interactive footer %q does not advertise Shift+PgUp/PgDn, the preview's own scroll key", interFooter)
	}
}
