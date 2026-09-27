package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
)

// This file is task 016/R155's own render coverage for the "Display"
// bullet (SPEC §3.4): a named profile's sidebar header line grows a
// "profile: <name> · " prefix in front of the socket half, eliding the
// socket half FIRST when the line does not fit; default's line stays
// byte-identical to its pre-phase-4e (d40a0552b) render, and only a named
// profile emits a terminal title.

// sidebarSocketHeaderEntry fetches the very first sidebarEntries() line at
// contentWidth -- the socket/profile header is always entries[0] when
// Socket != "" (tui.go's sidebarEntries), and every case below sets one.
func sidebarSocketHeaderEntry(t *testing.T, m Model, contentWidth int) string {
	t.Helper()
	entries := m.sidebarEntries(contentWidth)
	if len(entries) == 0 {
		t.Fatalf("sidebarEntries(%d) returned no entries", contentWidth)
	}
	return entries[0].text
}

// TestSidebarHeaderNamedProfileAtNormalWidth pins the PRD/SPEC's literal
// shape at a width with plenty of room: "profile: <name> \u00b7 socket:
// deck-<name>", nothing elided.
func TestSidebarHeaderNamedProfileAtNormalWidth(t *testing.T) {
	m := New(nil, config.Settings{Profile: "work", Socket: "deck-work"}, "")
	got := sidebarSocketHeaderEntry(t, m, 60)
	want := "profile: work \u00b7 socket: deck-work"
	if got != want {
		t.Fatalf("header at width 60 = %q, want %q", got, want)
	}
}

// TestSidebarHeaderNamedProfileAtNarrowestSidebarWidth pins the elision
// rule at the narrowest supported sidebar (SidebarWidthFloor == 24,
// side-by-side content width 21, per sidebarEntryContentWidth): the name
// half survives whole ("profile: work" is never touched), the socket half
// is what shrinks, and the whole line still fits the budget.
func TestSidebarHeaderNamedProfileAtNarrowestSidebarWidth(t *testing.T) {
	m := New(nil, config.Settings{Profile: "work", Socket: "deck-work"}, "")
	layout := LayoutResult{Effective: LayoutSideBySide, Sidebar: Rect{Width: SidebarWidthFloor}}
	contentWidth := sidebarEntryContentWidth(layout)
	if contentWidth <= 0 {
		t.Fatalf("sidebarEntryContentWidth at the sidebar floor = %d, want positive", contentWidth)
	}
	got := sidebarSocketHeaderEntry(t, m, contentWidth)
	if stringWidth(got) > contentWidth {
		t.Fatalf("header %q is %d cells wide, want <= %d", got, stringWidth(got), contentWidth)
	}
	if !strings.HasPrefix(got, "profile: work") {
		t.Fatalf("header %q does not keep the profile name whole at the narrowest sidebar width", got)
	}
	if strings.Contains(got, "socket: deck-work") {
		t.Fatalf("header %q kept the full socket half at the narrowest sidebar width, want it elided", got)
	}
}

// TestSidebarHeaderDefaultUnchanged pins default's header exactly as it
// rendered before this task (and on the pre-phase-4e d40a0552b tree): the
// literal "socket: deck" with no "profile:" segment at all, at both a
// normal width and the narrowest sidebar width.
func TestSidebarHeaderDefaultUnchanged(t *testing.T) {
	m := New(nil, config.Settings{Profile: config.DefaultProfile, Socket: "deck"}, "")
	for _, width := range []int{60, sidebarEntryContentWidth(LayoutResult{Effective: LayoutSideBySide, Sidebar: Rect{Width: SidebarWidthFloor}})} {
		got := sidebarSocketHeaderEntry(t, m, width)
		if got != "socket: deck" {
			t.Fatalf("default header at width %d = %q, want exactly %q", width, got, "socket: deck")
		}
	}
}

// TestTerminalTitleNamedProfileVsDefault asserts the title half of SPEC's
// "Display" bullet: Init() batches a tea.SetWindowTitle("deck: <name>")
// command for a named profile, and no title command at all for default.
func TestTerminalTitleNamedProfileVsDefault(t *testing.T) {
	named := New(nil, config.Settings{Profile: "work", Socket: "deck-work"}, "")
	if got, ok := collectWindowTitle(named.Init()); !ok || got != "deck: work" {
		t.Fatalf("named profile Init() window title = (%q, %v), want (%q, true)", got, ok, "deck: work")
	}

	def := New(nil, config.Settings{Profile: config.DefaultProfile, Socket: "deck"}, "")
	if got, ok := collectWindowTitle(def.Init()); ok {
		t.Fatalf("default Init() emitted a window title %q, want none", got)
	}
}

// collectWindowTitle runs a tea.Cmd (as returned by Init, a tea.Batch of
// several) all the way down to its leaf messages and reports the string
// value of the first one whose underlying kind is string -- the shape
// tea.SetWindowTitle's own (unexported) setWindowTitleMsg has. tea.Batch
// itself returns a batchMsg ([]tea.Cmd, also unexported) that this walks
// via reflection, since Update never runs here.
func collectWindowTitle(cmd tea.Cmd) (string, bool) {
	if cmd == nil {
		return "", false
	}
	msg := cmd()
	if msg == nil {
		return "", false
	}
	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.String {
		return v.String(), true
	}
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			sub, ok := elem.Interface().(tea.Cmd)
			if !ok {
				continue
			}
			if got, ok := collectWindowTitle(sub); ok {
				return got, true
			}
		}
	}
	return "", false
}
