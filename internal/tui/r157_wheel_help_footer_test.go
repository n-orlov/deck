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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
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

// TestFooterAdvertisesTheKeyTheSidebarWheelDuplicates proves the list-mode
// half of the footer's agreement: SPEC §11.8's rule is that every mouse
// binding duplicates a key, and for "wheel over the sidebar" that key is
// the unconditional ↑/↓ entry (footerLegend's first row), present in the
// list-mode footer whether or not a row is selected.
func TestFooterAdvertisesTheKeyTheSidebarWheelDuplicates(t *testing.T) {
	m := New(nil, config.Settings{Mouse: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	m.selected = rowCursor(0)

	footer := stripANSI(m.footerLine())
	if !strings.Contains(footer, "\u2191/\u2193") {
		t.Fatalf("list-mode footer %q does not advertise \u2191/\u2193, the key \"wheel over the sidebar\" duplicates (SPEC \u00a711.8's table)", footer)
	}
}

// interactiveFooterFor builds an interactive Model (no live grid) at the
// given width and scroll offset and renders footerLine -- the same entry
// point mainView uses, which routes interactive mode to
// interactiveFooterLine.
func interactiveFooterFor(mouse bool, width, offset int) string {
	m := New(nil, config.Settings{Mouse: mouse}, "")
	m.width, m.height = width, 30
	m.interactive = true
	m.setInteractiveScrollOffset(offset)
	return stripANSI(m.footerLine())
}

const r157SidebarWheelFooter = "sidebar wheel scrolls the list"

// TestInteractiveFooterDescribesTheSidebarWheel is R157's footer claim:
// while interactive, the footer itself says the wheel over the sidebar
// scrolls the list (SPEC §11.8: "over the sidebar it scrolls the list
// exactly as it does in list mode ... without leaving interactive mode";
// §11.9: "over the sidebar the wheel scrolls the list"), at the live
// bottom and while scrolled back, beside Ctrl+Q and never past the frame.
func TestInteractiveFooterDescribesTheSidebarWheel(t *testing.T) {
	cases := []struct {
		name          string
		width, offset int
	}{
		{"live bottom, 100 columns", 100, 0},
		{"live bottom, 160 columns", 160, 0},
		{"scrolled back, 200 columns", 200, 5},
	}
	for _, tc := range cases {
		footer := interactiveFooterFor(true, tc.width, tc.offset)
		if !strings.Contains(footer, r157SidebarWheelFooter) {
			t.Errorf("%s: interactive footer %q does not describe the sidebar wheel (want %q, SPEC \u00a711.8/\u00a711.9)", tc.name, footer, r157SidebarWheelFooter)
		}
		if !strings.Contains(footer, "Ctrl+Q leave interactive mode") {
			t.Errorf("%s: interactive footer %q lost Ctrl+Q -- the sidebar wheel does not leave interactive mode, and the footer must still name the way out", tc.name, footer)
		}
		if tc.offset > 0 && !strings.Contains(footer, "Shift+PgUp/PgDn") {
			t.Errorf("%s: interactive footer %q dropped the preview's own scroll key to make room for the sidebar wheel", tc.name, footer)
		}
		if got := stringWidth(footer); got > tc.width {
			t.Errorf("%s: interactive footer %q is %d columns wide, over the %d-column frame", tc.name, footer, got, tc.width)
		}
	}

	// The segment is the least essential one: at the 80-column floor it
	// drops before anything else, and the line never overflows.
	for _, offset := range []int{0, 5} {
		footer := interactiveFooterFor(true, 80, offset)
		if got := stringWidth(footer); got > 80 {
			t.Errorf("offset=%d: 80-column interactive footer %q is %d columns wide", offset, footer, got)
		}
		if !strings.Contains(footer, "Ctrl+Q") {
			t.Errorf("offset=%d: 80-column interactive footer %q lost Ctrl+Q", offset, footer)
		}
	}
}

// TestInteractiveFooterOmitsTheSidebarWheelWithMouseOff proves SPEC
// §11.3's "never lists a key that is not bound": with the mouse off,
// updateInteractive ignores every wheel event, so the footer must not
// advertise a sidebar wheel that does nothing.
func TestInteractiveFooterOmitsTheSidebarWheelWithMouseOff(t *testing.T) {
	for _, offset := range []int{0, 5} {
		footer := interactiveFooterFor(false, 200, offset)
		if strings.Contains(footer, "sidebar wheel") {
			t.Errorf("offset=%d: mouse-off interactive footer %q advertises the sidebar wheel, which is unbound with the mouse off", offset, footer)
		}
	}
}

// TestInteractiveFooterAndHelpAgreeOnTheSidebarWheel proves the footer
// and the `?` help say the same thing: the rendered help's Keys entry for
// ↵ and the interactive footer both state that the wheel over the sidebar
// scrolls the list.
func TestInteractiveFooterAndHelpAgreeOnTheSidebarWheel(t *testing.T) {
	footer := interactiveFooterFor(true, 160, 0)
	help := renderedHelpBullet(t, false, "\u21b5 enter interactive mode on the selected running session", "force-enter interactive mode")
	for _, want := range []string{"sidebar", "scrolls the list"} {
		if !strings.Contains(footer, want) {
			t.Errorf("interactive footer %q does not contain %q", footer, want)
		}
		if !strings.Contains(help, want) {
			t.Errorf("rendered \u21b5 help entry does not contain %q:\n%s", want, help)
		}
	}
}

// TestInteractiveViewFooterDescribesTheSidebarWheel renders the whole
// frame through View() in real interactive mode against a private tmux
// pane, so the footer the user sees -- not only interactiveFooterLine in
// isolation -- describes the sidebar wheel.
func TestInteractiveViewFooterDescribesTheSidebarWheel(t *testing.T) {
	m := New(nil, config.Settings{Mouse: true}, "")
	m.width, m.height = 120, 30
	socket := fmt.Sprintf("r157-wheel-footer-%d", time.Now().UnixNano())
	newQuietSelectionPane(t, socket, "deck_r157wheel", 80, 24)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-r157wheel-1", Name: "r157wheel", Slug: "r157wheel", Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive {
		t.Fatalf("enterInteractive did not enter interactive mode")
	}
	defer got.exitInteractive()

	footer := stripANSI(footerLineOf(got.View()))
	if !strings.Contains(footer, r157SidebarWheelFooter) {
		t.Fatalf("interactive View() footer %q does not describe the sidebar wheel (want %q)", footer, r157SidebarWheelFooter)
	}
	if !strings.Contains(footer, "Ctrl+Q") {
		t.Fatalf("interactive View() footer %q lost Ctrl+Q", footer)
	}
}
