package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestBareQWhileDetailOpenQuitsInsteadOfNoOp is task 079's regression test.
// Before this fix, updateDetailView had no case for "q" (or "ctrl+c"), so a
// bare q while the `i` detail dialog was open fell through the switch
// entirely and was silently swallowed -- helpText's own unconditional "q or
// Ctrl+C quit deck" line (Keys section) already promised otherwise, and
// updateHelpView already implements exactly this for the `?` overlay. This
// is what made features/agent_session.feature's login_shell scenario (and
// four siblings named by task 023: crash.feature's SIGKILL and
// pre_launch-fails scenarios, launch_lease.feature's changed-verdict-target
// scenario, permission_modes.feature's degraded-profile scenario) hang the
// black-box harness's "deck client exits cleanly" step (features/
// assertions_test.go's clientExitsCleanly sends one bare "q" and waits up
// to 5s for the process to exit): with m.detail still true, that "q" used
// to do nothing at all.
func TestBareQWhileDetailOpenQuitsInsteadOfNoOp(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{}, "")
			m.sessions = []store.Session{{ID: "s1", Name: "alpha"}}
			m.selected = 0
			m.detail = true

			next, cmd := m.updateDetailView(tc.msg)
			key := tc.name

			if cmd == nil {
				t.Fatalf("updateDetailView(%q) with m.detail=true returned a nil cmd; want tea.Quit (a bare %q used to be a silent no-op here)", key, key)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("updateDetailView(%q) with m.detail=true returned a cmd that is not tea.Quit", key)
			}
			nextModel := next.(Model)
			if !nextModel.detail {
				t.Fatalf("updateDetailView(%q) should leave m.detail untouched (tea.Quit itself ends the program, this handler never needs to close the dialog first); got m.detail=false", key)
			}
		})
	}
}
