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

func TestStartingCopyDistinguishesShellFromSignalledAgents(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "shell row", Agent: "shell", Status: "starting"},
		{Name: "claude row", Agent: "claude", Status: "starting"},
		{Name: "pi row", Agent: "pi", Status: "starting"},
	}
	view := model.View()
	var shellLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "shell row") {
			shellLine = line
			break
		}
	}
	if !strings.Contains(shellLine, "starting") || strings.Contains(shellLine, "awaiting signal") {
		t.Fatalf("shell starting copy is not plain: %q\n%s", shellLine, view)
	}
	if strings.Count(view, "starting · awaiting signal") != 2 {
		t.Fatalf("awaiting-signal copy should appear only on the two agent rows:\n%s", view)
	}

	model.sessions = model.sessions[:1]
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
