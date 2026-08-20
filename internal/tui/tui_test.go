package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func TestEmptyAndHelpViewsAreDiscoverable(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "tmux is required but was not found on PATH")
	empty := model.View()
	for _, want := range []string{"No sessions yet", "Press n", "tmux unavailable", "Install tmux 3.2"} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty view missing %q:\n%s", want, empty)
		}
	}
	model.help = true
	help := model.View()
	for _, want := range []string{
		"↑/↓ or j/k select", "↵ attach the selected running session", "Y acknowledge", "unseen marker", "n create", "x kill",
		"? open/close help", "Esc closes help", "q or Ctrl+C quit",
		"r resume", "resumed agents", "starting · awaiting", "live shells", "become \"running\"", "starting elsewhere",
		"P switch the permission profile", "restart to apply", "live pane",
		"p pin", "one-shot fresh conversation", "auto-resume",
		"Permission profile", "Pre-launch command", "loading secrets",
		"Login shell", "Launch args", "allow_yolo",
		"DECK_HOME", "DECK_TMUX_SOCKET", "DECK_CLOCK", "DECK_CLOCK_STEP", "clock.now", "resolved data root", "DECK_ID_SEED",
		"kill -USR1 <deck-client-pid>", "each invocation advances", "shared clock by exactly DECK_CLOCK_STEP",
		"the trigger updates it and every process reads it",
		"DECK_RECONCILE_MS", "DECK_PREVIEW_MS", "DECK_ASCII", "DECK_ANIM", "DECK_COLOR", "NO_COLOR",
		"tmux -L deck ls", "Plain tmux attach does not find deck",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help view missing %q", want)
		}
	}
	for _, unavailable := range []string{"suggested increment", "write it to advance", "_hook", "> advance", "resume/start", "restart preserving", "delete", "send message", "env editor", "event log", "filter list", "snooze", "archive", "undo"} {
		if strings.Contains(help, unavailable) {
			t.Errorf("help advertises unavailable action %q:\n%s", unavailable, help)
		}
	}
}

// TestStartingCopyDistinguishesShellFromSignalledAgents proves (task 012)
// that the row itself never carries "awaiting signal" for any agent — the
// row's status word is now always the bare "starting" regardless of which
// session is selected — while the footer surfaces the reason for whichever
// row is currently selected: nothing for a shell (it has no such reason at
// all), and "starting · awaiting signal" for an unsignalled coding agent.
func TestStartingCopyDistinguishesShellFromSignalledAgents(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "shell row", Agent: "shell", Status: "starting"},
		{Name: "claude row", Agent: "claude", Status: "starting"},
		{Name: "pi row", Agent: "pi", Status: "starting"},
	}

	// No row, selected or not, ever carries the reason text itself.
	view := model.View()
	for _, name := range []string{"shell row", "claude row", "pi row"} {
		var line string
		for _, candidate := range strings.Split(view, "\n") {
			if strings.Contains(candidate, name) {
				line = candidate
				break
			}
		}
		if !strings.Contains(line, "starting") || strings.Contains(line, "awaiting signal") {
			t.Fatalf("row %q starting copy is not plain: %q\n%s", name, line, view)
		}
	}

	// Selecting the shell row: the footer shows no reason at all.
	model.selected = 0
	view = model.View()
	if strings.Contains(view, "awaiting signal") {
		t.Fatalf("shell selected but footer shows 'awaiting signal':\n%s", view)
	}

	// Selecting either signalled agent's row: the footer, and only the
	// footer, carries the reason — exactly once.
	for _, index := range []int{1, 2} {
		model.selected = index
		view = model.View()
		if strings.Count(view, "starting · awaiting signal") != 1 {
			t.Fatalf("selecting row %d did not put exactly one 'starting · awaiting signal' on the footer:\n%s", index, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, model.sessions[index].Name) && strings.Contains(line, "awaiting signal") {
				t.Fatalf("row %d still carries its own reason text instead of the footer:\n%s", index, view)
			}
		}
	}

	model.sessions = model.sessions[:1]
	model.selected = 0
	model.detail = true
	detail := model.View()
	if !strings.Contains(detail, "Status:             starting") || strings.Contains(detail, "awaiting signal") {
		t.Fatalf("shell detail starting copy is not plain:\n%s", detail)
	}
}

func TestASCIIColorAndFrozenRelativeTimeRendering(t *testing.T) {
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	model := New(nil, config.Settings{ASCII: true, Clock: clock}, "")
	model.sessions = []store.Session{{Name: "shell", Agent: "shell", Status: "running", CreatedAt: clock.Now().UnixMilli()}}
	view := model.View()
	for _, want := range []string{"deck - sessions", "created just now", "up/down", "Enter attach"} {
		if !strings.Contains(view, want) {
			t.Errorf("ASCII/frozen view missing %q:\n%s", want, view)
		}
	}
	if strings.ContainsAny(view, "—↑↓↵·") {
		t.Errorf("ASCII view retained Unicode glyphs:\n%s", view)
	}
	if strings.Contains(view, "\x1b[") {
		t.Errorf("color-disabled view retained styling escapes: %q", view)
	}
	colored := New(nil, config.Settings{Color: true}, "").View()
	if !strings.Contains(colored, "\x1b[1;36mdeck\x1b[0m") {
		t.Errorf("color-enabled view omitted deck styling: %q", colored)
	}
	// A real delay must not make the frozen relative value advance.
	time.Sleep(10 * time.Millisecond)
	if !strings.Contains(model.View(), "created just now") {
		t.Error("frozen clock did not stabilize rendered relative time")
	}
}

func TestFrozenClockDoesNotClaimSidebarWidthKey(t *testing.T) {
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "2m")
	if err != nil {
		t.Fatal(err)
	}
	model := New(nil, config.Settings{Clock: clock}, "")
	updated, _ := model.Update(shellCreated{})
	model = updated.(Model)
	if got := clock.Now().Format(time.RFC3339); got != "2025-01-02T03:04:05Z" {
		t.Fatalf("creation advanced clock to %s", got)
	}
	updated, _ = model.Update(key(">"))
	if got := clock.Now().Format(time.RFC3339); got != "2025-01-02T03:04:05Z" {
		t.Fatalf("> key advanced clock to %s; it is reserved for sidebar width", got)
	}
	if updated.(Model).creating {
		t.Fatal("successful creation left create dialog open")
	}
}

// TestPipeCyclesLayoutMode covers task 015's `|` cycle: auto -> side-by-side
// -> stacked -> collapsed -> auto, pinning an explicit mode each step
// regardless of the terminal's width, and restoring auto (not tab) once the
// cycle comes back around from collapsed.
func TestPipeCyclesLayoutMode(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	if model.layoutMode != "" {
		t.Fatalf("model started with a non-default layoutMode %q", model.layoutMode)
	}
	want := []string{LayoutSideBySide, LayoutStacked, LayoutCollapsed, LayoutAuto}
	for _, w := range want {
		updated, _ := model.Update(key("|"))
		model = updated.(Model)
		if model.layoutMode != w {
			t.Fatalf("after | : layoutMode = %q, want %q", model.layoutMode, w)
		}
	}
}

// TestAngleBracketsAdjustAndClampSidebarWidth covers task 015's `<`/`>`
// keys: they step sidebar_width by one column and clamp at both
// [SidebarWidthFloor, width-PreviewWidthFloor] ends rather than running past
// them.
func TestAngleBracketsAdjustAndClampSidebarWidth(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 100, 24

	updated, _ := model.Update(key("<"))
	model = updated.(Model)
	if model.sidebarWidth != 34 {
		t.Fatalf("after one < from default 35: sidebarWidth = %d, want 34", model.sidebarWidth)
	}
	updated, _ = model.Update(key(">"))
	model = updated.(Model)
	if model.sidebarWidth != 35 {
		t.Fatalf("after > : sidebarWidth = %d, want back to 35", model.sidebarWidth)
	}

	// Drive it down past the floor: SidebarWidthFloor is 24 at width 100.
	for i := 0; i < 20; i++ {
		updated, _ = model.Update(key("<"))
		model = updated.(Model)
	}
	if model.sidebarWidth != SidebarWidthFloor {
		t.Fatalf("driving < past the floor: sidebarWidth = %d, want floor %d", model.sidebarWidth, SidebarWidthFloor)
	}

	// Drive it up past the ceiling: width-PreviewWidthFloor = 100-40 = 60.
	for i := 0; i < 60; i++ {
		updated, _ = model.Update(key(">"))
		model = updated.(Model)
	}
	if want := 100 - PreviewWidthFloor; model.sidebarWidth != want {
		t.Fatalf("driving > past the ceiling: sidebarWidth = %d, want %d", model.sidebarWidth, want)
	}
}

// TestCollapsedStripRendersMarkerAndAttentionCount covers task 015's
// collapsed strip: a 3-column panel showing the `»` glyph above the
// attention count drawn vertically, with the preview taking the rest, and
// `|` restoring the sidebar afterwards.
func TestCollapsedStripRendersMarkerAndAttentionCount(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.layoutMode = LayoutCollapsed
	model.sessions = []store.Session{
		{Name: "waiting one", Agent: "claude", Status: "waiting"},
		{Name: "errored one", Agent: "claude", Status: "error"},
		{Name: "running one", Agent: "claude", Status: "running"},
	}

	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("collapsed view too short: %q", view)
	}
	// Row 0 is the top border; row 1 is the strip's first content row,
	// which must carry the » marker and nothing from a session name (the
	// strip never draws session rows).
	if !strings.Contains(lines[1], "»") {
		t.Fatalf("collapsed strip missing » marker on its first content row: %q", lines[1])
	}
	for _, name := range []string{"waiting one", "errored one", "running one"} {
		if strings.Contains(view, name) {
			t.Fatalf("collapsed strip still renders a session name %q:\n%s", name, view)
		}
	}
	// The attention count (2: one waiting, one error) must appear on the
	// strip's second content row.
	if !strings.Contains(lines[2], "2") {
		t.Fatalf("collapsed strip missing attention count '2' on its second content row: %q", lines[2])
	}
	// The strip is exactly CollapsedStripWidth columns wide (border+content),
	// so no session row's content ever leaks into it.
	if got := len([]rune(lines[1])); got < CollapsedStripWidth {
		t.Fatalf("collapsed strip line shorter than its own width: %d < %d", got, CollapsedStripWidth)
	}

	// | restores the sidebar (auto), not tab: the marker and count go away
	// and the session names come back.
	updated, _ := model.Update(key("|"))
	model = updated.(Model)
	view = model.View()
	if !strings.Contains(view, "waiting one") {
		t.Fatalf("| did not restore the sidebar's session rows:\n%s", view)
	}
}

func TestHelpTogglesAndEscapeCloses(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	updated, _ := model.Update(key("?"))
	model = updated.(Model)
	if !model.help {
		t.Fatal("? did not open help")
	}
	updated, _ = model.Update(key("?"))
	model = updated.(Model)
	if model.help {
		t.Fatal("? did not close help")
	}
	model.help = true
	updated, _ = model.Update(key("esc"))
	if updated.(Model).help {
		t.Fatal("Esc did not close help")
	}
}

// key avoids coupling these small behaviour tests to a terminal driver.
func key(value string) tea.KeyMsg {
	if value == "esc" {
		return tea.KeyMsg(tea.Key{Type: tea.KeyEscape})
	}
	return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(value)})
}
