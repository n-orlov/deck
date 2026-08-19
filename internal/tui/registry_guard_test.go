package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// key is defined in tui_test.go and reused here to send synthetic key
// presses through Model.Update.

// guardAdapter is a throwaway agent.Adapter, distinct from shell/claude/pi,
// used to prove PRD requirement 1 ("adding an adapter never requires
// touching internal/tui") end-to-end: it must show up in the Agent field,
// offer exactly its own declared profiles, and produce a degradation
// reason for a requested profile it does not support, purely by virtue of
// being registered — with zero edits to internal/tui.
type guardAdapter struct{}

func (guardAdapter) Kind() string { return "zzz-guard-adapter" }
func (guardAdapter) Capabilities() agent.Caps {
	return agent.Caps{Profiles: []string{"safe", "edits"}}
}
func (guardAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (guardAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (guardAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}

// TestBlackBoxRegistrySwapNeedsNoTUIEdit proves (PRD requirement 1) that a
// registry whose adapter membership differs from the stock shell/claude/pi
// set — an extra kind present, "pi" absent — is fully reflected by the TUI
// without any internal/tui source change: the extra kind appears in the
// create modal's Agent field, the absent stock kind appears nowhere in the
// modal, and the extra kind's own declared profiles (plus an explicit
// degradation reason for a profile it does not support) are exactly what
// gets offered/shown.
// TestDefaultCreateAgentPrefersShellRegardlessOfSortOrder pins the rule in
// defaultCreateAgent: "shell" must be the create modal's opening default
// whenever it is registered, even when the registry's alphabetical
// Kinds() order would put some other adapter first (e.g. an adapter kind
// that sorts before "shell"). A prior regression (task 002/003) silently
// switched the default to registry.Kinds()[0] == "claude", which broke
// the shell-only create-session flow the cmd/deck PTY tests depend on;
// registry_guard_test.go's own case never pinned this because it never
// exercised a registry where shell was not already first. This test also
// covers the fallback: when "shell" is absent entirely, the default must
// be Kinds()[0].
func TestDefaultCreateAgentPrefersShellRegardlessOfSortOrder(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(guardAdapter{}) // Kind() "zzz-guard-adapter" sorts after shell, so add an adapter that sorts before it too
	registry.Register(aardvarkAdapter{})

	kinds := registry.Kinds()
	if kinds[0] == "shell" {
		t.Fatalf("test setup bug: want a registry where alphabetical order does not put shell first, got %v", kinds)
	}
	if got := defaultCreateAgent(kinds); got != "shell" {
		t.Fatalf("defaultCreateAgent(%v) = %q, want \"shell\" preferred over alphabetical order", kinds, got)
	}

	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	updated, _ := m.Update(key("n"))
	nm := updated.(Model)
	if !nm.creating {
		t.Fatalf("pressing 'n' did not open the create modal")
	}
	if nm.createAgent != "shell" {
		t.Fatalf("create modal opened with default agent %q, want \"shell\"", nm.createAgent)
	}

	noShell := agent.NewRegistry()
	noShell.Register(agent.NewClaude())
	noShell.Register(agent.NewPi())
	noShellKinds := noShell.Kinds()
	if got := defaultCreateAgent(noShellKinds); got != noShellKinds[0] {
		t.Fatalf("defaultCreateAgent(%v) = %q, want fallback to Kinds()[0] = %q when shell is absent", noShellKinds, got, noShellKinds[0])
	}
}

// aardvarkAdapter is a throwaway adapter whose Kind() sorts alphabetically
// before "shell", used to prove defaultCreateAgent does not just pick
// Kinds()[0].
type aardvarkAdapter struct{}

func (aardvarkAdapter) Kind() string { return "aardvark-guard-adapter" }
func (aardvarkAdapter) Capabilities() agent.Caps {
	return agent.Caps{Profiles: []string{"safe"}}
}
func (aardvarkAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (aardvarkAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (aardvarkAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}

func TestBlackBoxRegistrySwapNeedsNoTUIEdit(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(guardAdapter{}) // extra kind; "pi" deliberately omitted

	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	m.creating = true
	m.createName = "guard-session"
	m.createCWD = t.TempDir()
	m.createAgent = registry.Kinds()[0]
	m.createProfile = "safe"
	m.createField = 2

	kinds := registry.Kinds()
	for _, k := range kinds {
		if k == "pi" {
			t.Fatal("test setup bug: registry unexpectedly contains \"pi\"")
		}
	}
	view := m.createView()
	if !strings.Contains(view, "zzz-guard-adapter") {
		t.Fatalf("create modal does not list the extra registered kind:\n%s", view)
	}
	var agentLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Agent:") {
			agentLine = line
			break
		}
	}
	if agentLine == "" {
		t.Fatalf("create modal has no Agent field line:\n%s", view)
	}
	wantCycles := "cycles: " + strings.Join(kinds, ", ")
	if !strings.Contains(agentLine, wantCycles) {
		t.Fatalf("Agent field line = %q, want it to contain %q", agentLine, wantCycles)
	}
	if strings.Contains(agentLine, "pi") {
		t.Fatalf("Agent field line mentions the absent stock kind %q: %q", "pi", agentLine)
	}

	// Cycle to the extra kind and confirm only its own declared profiles
	// (never claude's, never a hardcoded literal) are offered.
	found := false
	for i := 0; i < len(registry.Kinds())+1; i++ {
		if m.createAgent == "zzz-guard-adapter" {
			found = true
			break
		}
		m.cycleCreateField(1)
	}
	if !found {
		t.Fatalf("left/right cycling never reached the extra registered kind; ended on %q", m.createAgent)
	}
	options := m.createProfileOptionsFor(m.createAgent, true)
	if strings.Join(options, ",") != "safe,edits" {
		t.Fatalf("createProfileOptionsFor(extra kind) = %v, want [safe edits]", options)
	}

	// A session created against the extra kind with an unsupported
	// requested profile ("plan", which guardAdapter does not declare) must
	// carry an explicit degradation reason, and the detail pane must show
	// it — derived purely from the registered adapter's Caps, not from any
	// internal/tui special-case for a known kind name.
	caps, applicable := m.agentCapabilities("zzz-guard-adapter")
	if !applicable {
		t.Fatal("agentCapabilities reported the extra registered kind as not applicable")
	}
	resolved, degraded, reason := caps.ResolveProfile("zzz-guard-adapter", "plan")
	if !degraded || resolved != "safe" || reason == "" {
		t.Fatalf("ResolveProfile(extra kind, plan) = (%q,%v,%q), want degraded to safe with a reason", resolved, degraded, reason)
	}

	session := store.Session{
		Name: "guard-session", Agent: "zzz-guard-adapter", Status: "starting",
		PermissionProfile:       resolved,
		PermissionProfileReason: reason,
	}
	m.sessions = []store.Session{session}
	m.selected = 0
	m.creating = false
	m.detail = true
	detailView := m.View()
	if !strings.Contains(detailView, "degraded") || !strings.Contains(detailView, reason) {
		t.Fatalf("detail view missing the extra kind's degradation reason:\n%s", detailView)
	}
}
