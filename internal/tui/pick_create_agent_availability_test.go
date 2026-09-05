package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestPickCreateAgentFallsBackWhenLastUsedIsUnavailable covers task 007's
// fix directly: pickCreateAgent (exercised end-to-end through the "n" key
// handler, exactly as a real create-modal open does) must choose over the
// AVAILABLE set, not just the registered set. A lastCreateAgent that is
// still registered but not currently available (its injected prober
// leaves it out of the set) must fall back to defaultCreateAgent computed
// over that same available set and report lastUsed=false -- never open on
// an agent the modal cannot actually offer. The positive twin proves the
// remembered kind is still honoured, with the "(last used)" label, once
// it is back in the available set.
func TestPickCreateAgentFallsBackWhenLastUsedIsUnavailable(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	if err := db.SetLastCreateAgent(context.Background(), "pi"); err != nil {
		t.Fatal(err)
	}

	// pi absent from the injected available set: shell/claude are
	// registered and available, pi is registered but not available.
	m := New(db, config.Settings{}, "")
	if m.lastCreateAgent != "pi" {
		t.Fatalf("New() did not read the persisted last-create-agent: got %q", m.lastCreateAgent)
	}
	m = m.WithAvailableAgentKindsProber(func() []string { return []string{"shell", "claude"} })

	updated, _ := m.Update(key("n"))
	nm := updated.(Model)

	if nm.createAgentLastUsed {
		t.Fatal("createAgentLastUsed is true for a remembered kind absent from the available set")
	}
	if nm.createAgent != defaultCreateAgent(nm.createAvailableAgentKinds) {
		t.Fatalf("createAgent = %q, want defaultCreateAgent's fallback over the available set %q",
			nm.createAgent, defaultCreateAgent(nm.createAvailableAgentKinds))
	}
	if nm.createAgent != "shell" {
		t.Fatalf("createAgent = %q, want %q (defaultCreateAgent prefers shell)", nm.createAgent, "shell")
	}

	row := agentFieldRow(t, nm)
	if !strings.HasPrefix(row.value, "shell ") {
		t.Fatalf("Agent row value = %q, want it to read %q", row.value, "shell")
	}
	if strings.Contains(row.help, "(last used)") {
		t.Fatalf("Agent row help = %q, want no \"(last used)\" label when the remembered kind is unavailable", row.help)
	}

	// pi present in the injected available set: the remembered kind is
	// still honoured, with the label.
	m2 := New(db, config.Settings{}, "")
	m2 = m2.WithAvailableAgentKindsProber(func() []string { return []string{"shell", "claude", "pi"} })

	updated2, _ := m2.Update(key("n"))
	nm2 := updated2.(Model)

	if !nm2.createAgentLastUsed {
		t.Fatal("createAgentLastUsed is false for a remembered kind present in the available set")
	}
	if nm2.createAgent != "pi" {
		t.Fatalf("createAgent = %q, want %q", nm2.createAgent, "pi")
	}
	row2 := agentFieldRow(t, nm2)
	if !strings.HasPrefix(row2.value, "pi ") {
		t.Fatalf("Agent row value = %q, want it to read %q", row2.value, "pi")
	}
	if !strings.Contains(row2.help, "(last used)") {
		t.Fatalf("Agent row help = %q, want it to contain %q", row2.help, "(last used)")
	}
}
