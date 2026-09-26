package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// renderedHelpBullet opens the `?` overlay through the real key path on a
// frame tall and wide enough that framedDialogScrollable shows the whole
// help body at once (no wrap, no scroll window), renders View(), and
// returns the bullet that starts with marker -- its first rendered line
// plus every continuation line up to (not including) the rendered line
// starting with next -- as one whitespace-normalised string. It reads the
// RENDERED frame, not helpText, so a bullet the overlay dropped, wrapped
// away or never drew fails here even if helpText still carried it.
func renderedHelpBullet(t *testing.T, ascii bool, marker, next string) string {
	t.Helper()
	m := New(nil, config.Settings{Mouse: true, ASCII: ascii}, "")
	m.width, m.height = 220, 1200
	updated, _ := m.Update(key("?"))
	m = updated.(Model)
	if !m.help {
		t.Fatalf("? did not open the help overlay")
	}
	lines := strings.Split(stripANSI(m.View()), "\n")
	body := make([]string, len(lines))
	for i, l := range lines {
		// Strip the overlay's own border column(s) on either side so
		// each line reads as the help body's own text.
		l = strings.TrimSpace(l)
		l = strings.Trim(l, "│|")
		body[i] = strings.TrimSpace(l)
	}
	start := -1
	for i, l := range body {
		if strings.HasPrefix(l, marker) {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("rendered ? help (ascii=%v) has no line starting with %q:\n%s", ascii, marker, strings.Join(body, "\n"))
	}
	end := len(body)
	for i := start + 1; i < len(body); i++ {
		if strings.HasPrefix(body[i], next) {
			end = i
			break
		}
	}
	return strings.Join(strings.Fields(strings.Join(body[start:end], " ")), " ")
}

// TestRenderedHelpAndFooterNameTheKeysOfSection118sNewRows is R148's
// (task 026) render-level agreement check between SPEC §11.8's amended
// table and what deck actually draws. The table's rows the amendment
// (03752caf) changed or added are:
//
//	click a sidebar row                          -> selects AND enters interactive (↑/↓ then ↵)
//	click the passive preview                    -> ↵ on the already-selected row
//	click empty sidebar space while interactive  -> Ctrl+Q
//
// The `?` help must name each gesture next to the key it duplicates, and
// must no longer describe the pre-reversal behaviour (a click that only
// selects, a double-click that enters). The footer must advertise each
// duplicated key in the mode where the gesture fires: ↵ interactive in
// list mode (the sidebar-row and passive-preview clicks) and Ctrl+Q while
// interactive (the empty-sidebar click).
func TestRenderedHelpAndFooterNameTheKeysOfSection118sNewRows(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		enter := "↵"
		if ascii {
			enter = "Enter"
		}

		row := renderedHelpBullet(t, ascii, "click a sidebar row", "click a group header")
		for _, want := range []string{"select it and enter interactive mode", "↑/↓ then " + enter, "re-targets interactive mode", "Ctrl+Q"} {
			if !strings.Contains(row, want) {
				t.Errorf("ascii=%v: rendered sidebar-row bullet does not name %q (SPEC §11.8: selects that row and enters interactive, like ↑/↓ then ↵):\n%s", ascii, want, row)
			}
		}
		if strings.Contains(row, "preview follows on") {
			t.Errorf("ascii=%v: rendered sidebar-row bullet still describes the pre-reversal select-only click:\n%s", ascii, row)
		}

		preview := renderedHelpBullet(t, ascii, "click over the preview", "click empty sidebar space")
		for _, want := range []string{"interactive mode on the current selection", "(like " + enter + ")"} {
			if !strings.Contains(preview, want) {
				t.Errorf("ascii=%v: rendered passive-preview-click bullet does not name %q (SPEC §11.8: ↵ on the already-selected row):\n%s", ascii, want, preview)
			}
		}

		empty := renderedHelpBullet(t, ascii, "click empty sidebar space", "wheel over the preview")
		for _, want := range []string{"while interactive, leaves interactive mode", "(like Ctrl+Q)", "without moving the selection"} {
			if !strings.Contains(empty, want) {
				t.Errorf("ascii=%v: rendered empty-sidebar-click bullet does not name %q (SPEC §11.8: Ctrl+Q):\n%s", ascii, want, empty)
			}
		}

		m := New(nil, config.Settings{Mouse: true, ASCII: ascii}, "")
		m.width, m.height = 220, 1200
		updated, _ := m.Update(key("?"))
		m = updated.(Model)
		for _, l := range strings.Split(stripANSI(m.View()), "\n") {
			if strings.Contains(l, "double-click") {
				t.Errorf("ascii=%v: rendered help still advertises a double-click gesture SPEC §11.8's table no longer has: %q", ascii, strings.TrimSpace(l))
			}
		}

		list := New(nil, config.Settings{Mouse: true, ASCII: ascii}, "")
		list.width, list.height = 200, 30
		list.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
		list.selected = rowCursor(0)
		listFooter := lastRenderedLine(stripANSI(list.View()))
		if !strings.Contains(listFooter, enter+" interactive") {
			t.Errorf("ascii=%v: list-mode footer %q does not advertise %q, the key the sidebar-row and passive-preview clicks duplicate", ascii, listFooter, enter+" interactive")
		}

		inter := list
		inter.interactive = true
		interFooter := stripANSI(inter.footerLine())
		if !strings.Contains(interFooter, "Ctrl+Q leave interactive mode") {
			t.Errorf("ascii=%v: interactive footer %q does not advertise Ctrl+Q, the key the empty-sidebar click duplicates", ascii, interFooter)
		}
	}
}

// lastRenderedLine is the frame's last non-blank line -- the footer row.
func lastRenderedLine(frame string) string {
	lines := strings.Split(frame, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
