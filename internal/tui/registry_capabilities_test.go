package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// capsAdapter is a throwaway agent.Adapter that declares its own custom
// permission profile set ("lockdown", "open" — names that appear nowhere in
// internal/tui source) plus AssignsConversationID/Resumable, so a test can
// prove capability-dependent behaviour (profile options/badge, the `P`
// profile switch and `p` pin dialogs) is derived purely from the registry,
// never from a hardcoded "claude"/"pi" switch (PRD requirement 1).
type capsAdapter struct{}

func (capsAdapter) Kind() string { return "zzz-caps-adapter" }
func (capsAdapter) Capabilities() agent.Caps {
	return agent.Caps{
		Profiles:              []string{"lockdown", "open"},
		AssignsConversationID: true,
		Resumable:             true,
	}
}
func (capsAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (capsAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }

// newModelWithSessionAndRegistry builds a Model wired to registry (task 001
// constructor) with a single session already loaded and both the P and p
// dialogs' backing functions wired to no-op stubs, so key handling that
// gates on "is a callback configured" proceeds.
func newModelWithSessionAndRegistry(t *testing.T, registry *agent.Registry, session store.Session) Model {
	t.Helper()
	profileSwitch := func(_ context.Context, _, _ string) (store.Session, error) { return session, nil }
	resumeMode := func(_ context.Context, _, _ string) (store.Session, error) { return session, nil }
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		profileSwitch, resumeMode, nil, registry,
	)
	m.sessions = []store.Session{session}
	m.selected = 0
	return m
}

// TestRegistryDrivenCapabilities_ProfileBadgeCreateAndPin proves that
// profile-badge rendering, the create modal's offered profiles, the `P`
// permission-profile switch dialog, and the `p` pin dialog all reflect a
// registry-declared adapter's *own* capabilities (SPEC §5/§8), with no
// internal/tui edit needed to support it (PRD requirement 1).
func TestRegistryDrivenCapabilities_ProfileBadgeCreateAndPin(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(capsAdapter{})

	session := store.Session{
		ID:                "s1",
		Name:              "caps-session",
		Slug:              "caps-session",
		Agent:             "zzz-caps-adapter",
		Status:            "running",
		PermissionProfile: "lockdown",
		ResumeState:       "auto",
	}

	m := newModelWithSessionAndRegistry(t, registry, session)

	// The badge renders the adapter's own permission profile rather than
	// omitting it as "not applicable" (that only happens for adapters that
	// declare zero profiles, like shell).
	if badge := m.profileBadge(session); badge != "[lockdown]" {
		t.Fatalf("profileBadge = %q, want [lockdown]", badge)
	}

	// createProfileOptionsFor must offer exactly the adapter's declared
	// profiles, in declared order, never a hardcoded claude/pi list.
	options := m.createProfileOptionsFor(session.Agent, true)
	if strings.Join(options, ",") != "lockdown,open" {
		t.Fatalf("createProfileOptionsFor = %v, want [lockdown open]", options)
	}

	// The `P` profile-switch dialog must open (the adapter is applicable)
	// and its view must list only the adapter's declared profiles.
	updated, _ := m.Update(key("P"))
	m = updated.(Model)
	if !m.profileSwitching {
		t.Fatal("P did not open the profile switch dialog for a registry-only adapter")
	}
	view := m.profileSwitchView()
	if !strings.Contains(view, "lockdown, open") {
		t.Fatalf("profileSwitchView did not list the adapter's declared profiles:\n%s", view)
	}

	// Cycling right in the dialog must reach "open" without ever hitting a
	// profile the adapter did not declare.
	updated, _ = m.Update(key("right"))
	m = updated.(Model)
	if m.profileSwitchValue != "open" {
		t.Fatalf("profileSwitchValue after cycling = %q, want %q", m.profileSwitchValue, "open")
	}

	// Reset and exercise the `p` pin dialog: it must open because the
	// adapter declares AssignsConversationID, purely via the registry.
	m.profileSwitching = false
	updated, _ = m.Update(key("p"))
	m = updated.(Model)
	if !m.pinning {
		t.Fatal("p did not open the pin dialog for a registry adapter that AssignsConversationID")
	}
}
