package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
)

// throwawayAdapter is a stub agent.Adapter whose Kind() names nothing that
// exists anywhere in internal/tui source, so any appearance of it in the
// create modal must have come from the registry, not a hardcoded literal
// (PRD requirement 1).
type throwawayAdapter struct{}

func (throwawayAdapter) Kind() string { return "zzz-throwaway" }
func (throwawayAdapter) Capabilities() agent.Caps {
	return agent.Caps{}
}
func (throwawayAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (throwawayAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (throwawayAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}
func (throwawayAdapter) Probe(string) (string, string) { return "", "" }

// newModelWithRegistry builds a Model wired to registry via the
// registry-accepting constructor (task 001), with the create modal open.
func newModelWithRegistry(t *testing.T, registry *agent.Registry) Model {
	t.Helper()
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	m.creating = true
	m.createName = "my session"
	m.createCWD = t.TempDir()
	m.createAgent = registry.Kinds()[0]
	m.createProfile = "safe"
	m.createField = 2
	return m
}

// TestCreateModalAgentFieldDerivesFromRegistry proves the create modal's
// Agent field lists exactly the registered kinds (a throwaway kind unknown
// to internal/tui source), and that left/right cycling can reach it,
// without any edit to internal/tui (PRD requirement 1).
func TestCreateModalAgentFieldDerivesFromRegistry(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(throwawayAdapter{})

	m := newModelWithRegistry(t, registry)

	view := m.createView()
	if !strings.Contains(view, "zzz-throwaway") {
		t.Fatalf("create modal view does not mention throwaway adapter kind:\n%s", view)
	}

	// Cycle right through the field until we land on the throwaway kind,
	// bounded by the number of registered kinds so a broken cycle can't
	// spin forever.
	found := false
	for i := 0; i < len(registry.Kinds())+1; i++ {
		if m.createAgent == "zzz-throwaway" {
			found = true
			break
		}
		m.cycleCreateField(1)
	}
	if !found {
		t.Fatalf("left/right cycling on the Agent field never reached the throwaway kind; ended on %q", m.createAgent)
	}
}
