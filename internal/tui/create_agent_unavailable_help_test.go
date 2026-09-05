package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestCreateAgentHelpNamesUnavailableKinds covers task 008: the Agent
// row's help must name the registered kinds the injected available-set
// prober left out (derived from m.registry().Kinds() minus
// m.createAvailableAgentKinds), and must fall back to the exact pre-change
// literal, byte for byte, when nothing is missing -- registry().Kinds()'s
// stock shell/claude/pi order is only shell/claude/pi here since
// defaultAgentRegistry registers exactly those three adapters in that
// order.
func TestCreateAgentHelpNamesUnavailableKinds(t *testing.T) {
	db := openStoreForLastCreateAgent(t)

	// Only shell is available: claude and pi (the registry's other two
	// kinds, in registry order) must be named as not on PATH.
	m := New(db, config.Settings{}, "")
	m = m.WithAvailableAgentKindsProber(func() []string { return []string{"shell"} })
	updated, _ := m.Update(key("n"))
	nm := updated.(Model)

	row := agentFieldRow(t, nm)
	want := "which coding agent adapter launches this session; not on PATH: claude, pi"
	if row.help != want {
		t.Fatalf("Agent row help = %q, want %q", row.help, want)
	}

	// Every registered kind available: help is byte-identical to the
	// pre-change literal, with no "not on PATH" clause at all.
	m2 := New(db, config.Settings{}, "")
	m2 = m2.WithAvailableAgentKindsProber(func() []string { return []string{"shell", "claude", "pi"} })
	updated2, _ := m2.Update(key("n"))
	nm2 := updated2.(Model)

	row2 := agentFieldRow(t, nm2)
	wantAll := "which coding agent adapter launches this session"
	if row2.help != wantAll {
		t.Fatalf("Agent row help = %q, want the pre-change literal %q byte-identical", row2.help, wantAll)
	}
}
