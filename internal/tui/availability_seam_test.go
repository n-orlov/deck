package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
)

// fullRegistryForAvailabilitySeamTest builds the stock shell/claude/pi
// registry the availability seam is exercised against below: the injected
// prober narrows what is *available*, never what is *registered*, so the
// cycle set below must always be a subset of this full registry's Kinds().
func fullRegistryForAvailabilitySeamTest() *agent.Registry {
	r := agent.NewRegistry()
	r.Register(agent.NewShell())
	r.Register(agent.NewClaude())
	r.Register(agent.NewPi())
	return r
}

// agentRowText extracts the Agent field's own text line from a create-modal
// view, the same way registry_guard_test.go's TestBlackBoxRegistrySwapNeedsNoTUIEdit
// does, so assertions here read the actual rendered row rather than
// internal state.
func agentRowText(t *testing.T, view string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Agent:") {
			return line
		}
	}
	t.Fatalf("create modal has no Agent field line:\n%s", view)
	return ""
}

// TestAvailabilityProberInjectedOnOpenGatesCycle proves PRD R112 end to end
// through the public seam this task adds: WithAvailableAgentKindsProber
// supplies availability as kind-name data, Model.Update(key("n")) computes
// it exactly once and holds it in model state, and the Agent field's
// left/right cycle (cycleCreateField case 2, via updateCreate) — and the
// Agent row createFieldRows renders — never shows a kind outside the
// injected set, even though the registry backing the model has strictly
// more kinds registered than are available.
func TestAvailabilityProberInjectedOnOpenGatesCycle(t *testing.T) {
	registry := fullRegistryForAvailabilitySeamTest()

	cases := []struct {
		name      string
		available []string
	}{
		{name: "shell-only", available: []string{"shell"}},
		{name: "shell-and-claude", available: []string{"shell", "claude"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			available := append([]string(nil), tc.available...)
			m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
				nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
			)
			m = m.WithAvailableAgentKindsProber(func() []string {
				return append([]string(nil), available...)
			})
			m.width = 100

			updated, _ := m.Update(key("n"))
			nm := updated.(Model)
			if !nm.creating {
				t.Fatalf("pressing 'n' did not open the create modal")
			}
			if got := nm.createAvailableAgentKinds; strings.Join(got, ",") != strings.Join(available, ",") {
				t.Fatalf("createAvailableAgentKinds after 'n' = %v, want %v", got, available)
			}
			// Field navigation is ↑/↓ (applyDialogContract); focus the Agent
			// field (index 2: Name, Working directory, Agent) directly, the
			// same way registry_guard_test.go's own black-box test does,
			// rather than depending on how many ↓ presses it takes to get
			// there.
			nm.createField = 2

			// Cycle strictly more times than there are available kinds, so
			// every position in the injected set is visited at least once,
			// and confirm every value landed on -- both from the cycle
			// itself and from the rendered row -- is a member of the
			// injected set, never a registered-but-unavailable kind such
			// as "pi" (registered in every case above, available in
			// none).
			seen := map[string]bool{}
			for i := 0; i < len(registry.Kinds())+2; i++ {
				if !contains(available, nm.createAgent) {
					t.Fatalf("cycle landed on %q, which is outside the injected available set %v", nm.createAgent, available)
				}
				seen[nm.createAgent] = true

				view := nm.createView()
				row := agentRowText(t, view)
				for _, forbidden := range registry.Kinds() {
					if contains(available, forbidden) {
						continue
					}
					if strings.Contains(row, forbidden) {
						t.Fatalf("Agent row mentions unavailable registered kind %q: %q", forbidden, row)
					}
				}

				updated, _ = nm.Update(key("right"))
				nm = updated.(Model)
			}
			for _, want := range available {
				if !seen[want] {
					t.Fatalf("cycling never visited injected available kind %q; visited %v", want, seen)
				}
			}
		})
	}
}
