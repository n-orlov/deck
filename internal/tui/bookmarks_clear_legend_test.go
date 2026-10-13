package tui

import (
	"strings"
	"testing"
)

// R288: with the bookmarks setting on, the footer and the `?` overlay both
// list the Alt+M, Delete clear action; with it off neither mentions it.
func TestBookmarkClearLegendInFooterAndHelp(t *testing.T) {
	for _, on := range []bool{true, false} {
		m, _ := bookmarkKeyModel(t, on)
		m.width, m.height = 220, 30
		footer := strings.ToLower(stripANSI(m.footerLine()))
		help := strings.ToLower(stripANSI(m.styledHelpText()))
		if on {
			if !strings.Contains(footer, "alt+m del") || !strings.Contains(footer, "clear bookmark") {
				t.Fatalf("bookmarks on: footer lacks the clear action: %q", footer)
			}
			if !strings.Contains(help, "alt+m then delete clears") {
				t.Fatalf("bookmarks on: help lacks the clear action:\n%s", help)
			}
			continue
		}
		for _, text := range []string{footer, help} {
			if strings.Contains(text, "clear bookmark") || strings.Contains(text, "alt+m del") || strings.Contains(text, "clears every bookmark") {
				t.Fatalf("bookmarks off: the clear action is mentioned: %q", text)
			}
		}
	}
}
