package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
)

// TestCreateModalDegradesRequestedProfileThroughResolveProfile pins the create
// modal's degrade behaviour (SPEC §5: "Unsupported profiles degrade to the
// nearest safe one and say so ... rather than silently lying") to the generic
// agent.Caps.ResolveProfile call, using codex's real adapter and its real
// missing `plan` profile (R127). Both halves of what the user sees -- the
// value the Permission profile field falls back to and the sentence
// explaining why -- are compared against what ResolveProfile itself returns
// rather than against literals held in this package, so a regression in that
// generic path (a different fallback target, a dropped reason) fails here
// instead of quietly passing against a copy of its old wording.
func TestCreateModalDegradesRequestedProfileThroughResolveProfile(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewCodex())

	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	// Wide enough that framedDialog's fixed box (task 030) keeps the
	// degrade sentence on one physical line, so this test asserts the
	// sentence itself rather than a wrap-tolerant pattern.
	m.width = 120
	m = m.WithAvailableAgentKindsProber(func() []string { return registry.Kinds() })
	m.createAvailableAgentKinds = m.computeAvailableAgentKinds()
	m.creating = true
	m.createName = "codex-plan"
	m.createCWD = t.TempDir()
	m.createAgent = "claude"
	m.createProfile = "safe"

	// Request "plan" the way a user does: cycle the Permission profile
	// field (field 3) itself, which is what marks the value as an explicit
	// request rather than a default.
	m.createField = 3
	for i := 0; i < len(createProfileOptions)+1 && m.createProfile != "plan"; i++ {
		m.cycleCreateField(1)
	}
	if m.createProfile != "plan" {
		t.Fatalf("cycling the Permission profile field never reached %q on claude; ended on %q", "plan", m.createProfile)
	}

	// Then pick the agent: cycle Agent (field 2) onto codex, which does not
	// declare "plan" at all (internal/agent/codex.go's codexProfiles).
	m.createField = 2
	for i := 0; i < len(m.createAvailableAgentKinds)+1 && m.createAgent != "codex"; i++ {
		m.cycleCreateField(1)
	}
	if m.createAgent != "codex" {
		t.Fatalf("cycling the Agent field never reached %q; ended on %q", "codex", m.createAgent)
	}

	caps := agent.NewCodex().Capabilities()
	resolved, degraded, reason := caps.ResolveProfile("codex", "plan")
	if !degraded {
		t.Fatal("test premise broken: codex reports it supports permission profile \"plan\"")
	}
	if m.createProfile != resolved {
		t.Fatalf("create modal's Permission profile = %q after switching to codex, want ResolveProfile's own %q", m.createProfile, resolved)
	}
	if note := m.createProfileDegradeNote(); note != reason {
		t.Fatalf("createProfileDegradeNote() = %q, want ResolveProfile's own reason %q", note, reason)
	}
	// Both render paths must carry it: the plain body scroll math measures
	// (createBody) and the themed one the view actually draws (createView ->
	// styledCreateBody).
	if body := m.createBody(); !strings.Contains(body, reason) {
		t.Fatalf("create modal's plain body does not state the degradation %q:\n%s", reason, body)
	}
	view := m.createView()
	if !strings.Contains(stripANSI(view), reason) {
		t.Fatalf("create modal's rendered view does not state the degradation %q:\n%s", reason, view)
	}

	// Switching back to an agent that does declare "plan" leaves nothing to
	// explain: the note is derived live from the selected adapter's caps,
	// never a sticky string.
	for i := 0; i < len(m.createAvailableAgentKinds)+1 && m.createAgent != "claude"; i++ {
		m.cycleCreateField(1)
	}
	if m.createAgent != "claude" {
		t.Fatalf("cycling the Agent field never returned to %q; ended on %q", "claude", m.createAgent)
	}
	if note := m.createProfileDegradeNote(); note != "" {
		t.Fatalf("createProfileDegradeNote() = %q on claude, want no note (claude declares plan)", note)
	}
	if body := m.createBody(); strings.Contains(body, "does not support permission profile") {
		t.Fatalf("create modal still states a degradation after switching back to claude:\n%s", body)
	}
}
