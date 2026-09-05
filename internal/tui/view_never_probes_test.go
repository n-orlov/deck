package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestViewNeverProbesAfterOpen proves task 009's guard: the "n" handler
// populates m.createAvailableAgentKinds by calling the injected prober
// exactly once (task 005/PRD R112), and every subsequent read of that
// availability -- pickCreateAgent, createUnavailableAgentKinds,
// createAgentHelp, createFieldRows, View() itself -- must consult the
// already-populated field rather than falling back to
// computeAvailableAgentKinds() and re-invoking the prober. A prober that
// hits the filesystem (a PATH probe) on every keystroke/render would be a
// perceptible stutter in the real TUI; this test makes that regression a
// hard failure instead of a vibe.
func TestViewNeverProbesAfterOpen(t *testing.T) {
	db := openStoreForLastCreateAgent(t)

	calls := 0
	prober := func() []string {
		calls++
		return []string{"shell", "claude", "pi"}
	}

	m := New(db, config.Settings{}, "")
	m = m.WithAvailableAgentKindsProber(prober)

	updated, _ := m.Update(key("n"))
	nm := updated.(Model)
	if !nm.creating {
		t.Fatal("pressing \"n\" did not open the create modal")
	}
	if calls != 1 {
		t.Fatalf("prober call count after opening the modal = %d, want exactly 1", calls)
	}

	for i := 0; i < 20; i++ {
		_ = nm.View()
	}

	if calls != 1 {
		t.Fatalf("prober call count after 20 renders = %d, want it unchanged at 1 (View must not re-probe)", calls)
	}
}
