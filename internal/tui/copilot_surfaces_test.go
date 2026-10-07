package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func allKindsRegistry() *agent.Registry {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewCodex())
	registry.Register(agent.NewCopilot())
	return registry
}

// openCreateDialog opens the real create dialog ("n") on a model whose registry
// holds every shipped kind and whose availability prober reports `available`.
func openCreateDialog(t *testing.T, settings config.Settings, available []string) Model {
	t.Helper()
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		openStoreForLastCreateAgent(t), settings, "", nil, nil, nil, nil, nil, nil, nil, nil, allKindsRegistry(),
	)
	m.width, m.height = 120, 40
	m = m.WithAvailableAgentKindsProber(func() []string { return available })
	updated, _ := m.Update(key("n"))
	return updated.(Model)
}

func cycleAgentTo(t *testing.T, m Model, kind string) Model {
	t.Helper()
	m.createField = 2
	for i := 0; i < len(m.createAvailableAgentKinds)+1 && m.createAgent != kind; i++ {
		m.cycleCreateField(1)
	}
	if m.createAgent != kind {
		t.Fatalf("cycling the Agent field never reached %q (offers %v); ended on %q", kind, m.createAvailableAgentKinds, m.createAgent)
	}
	return m
}

// profilesByCycling returns every distinct Permission profile value the dialog's
// own Permission profile field shows when cycled right a full lap.
func profilesByCycling(m Model) []string {
	m.createField = 3
	var seen []string
	for i := 0; i < 8; i++ {
		if !contains(seen, m.createProfile) {
			seen = append(seen, m.createProfile)
		}
		m.cycleCreateField(1)
	}
	return seen
}

// R220.1: the create dialog offers copilot, and for it exactly safe, edits, yolo.
func TestCreateDialogOffersCopilotWithSafeEditsAndYolo(t *testing.T) {
	m := openCreateDialog(t, config.Settings{AllowYolo: true}, []string{"claude", "codex", "copilot", "pi", "shell"})
	if !contains(m.createAvailableAgentKinds, "copilot") {
		t.Fatalf("Agent field offers %v, want copilot among them", m.createAvailableAgentKinds)
	}
	m = cycleAgentTo(t, m, "copilot")
	if !strings.Contains(m.createView(), "Agent: copilot") {
		t.Fatalf("create dialog does not show %q:\n%s", "Agent: copilot", m.createView())
	}
	want := []string{"safe", "edits", "yolo"}
	if got := m.createProfileOptionsFor("copilot", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("createProfileOptionsFor(copilot, allow_yolo) = %v, want %v", got, want)
	}
	if got := profilesByCycling(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("Permission profile field cycles %v on copilot, want exactly %v", got, want)
	}
	if view := m.createView(); !strings.Contains(view, "Permission profile") {
		t.Fatalf("create dialog does not render the Permission profile field:\n%s", view)
	}
	// allow_yolo = false removes yolo for copilot exactly as it does for every kind.
	if got := m.createProfileOptionsFor("copilot", false); !reflect.DeepEqual(got, []string{"safe", "edits"}) {
		t.Fatalf("createProfileOptionsFor(copilot, !allow_yolo) = %v, want [safe edits]", got)
	}
}

// R220.2: this product's session row carries no agent-kind text at all (the
// sidebar omits it by design, see sidebarRowLines); what identifies a copilot
// row is its permission-profile badge, and the `i` detail pane carries the
// Agent line. Both are asserted for copilot.
func TestCopilotRowBadgeAndDetailAgentLineRenderCopilot(t *testing.T) {
	session := store.Session{
		ID: "s1", Name: "cop", Slug: "cop", Agent: "copilot", CWD: "/repo/cop",
		Status: "running", PermissionProfile: "edits", ResumeState: "auto",
	}
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, allKindsRegistry(),
	)
	m.width, m.height = 120, 40
	m.sessions = []store.Session{session}
	m.selected = rowCursor(0)

	lines, _, _ := m.sidebarRowLines(0, session, false)
	row := strings.Join(lines, "\n")
	if !strings.Contains(row, "cop") || !strings.Contains(row, "[edits]") {
		t.Fatalf("copilot row lacks its name or its [edits] profile badge:\n%s", row)
	}
	if got := m.profileBadge(session); got != "[edits]" {
		t.Fatalf("profileBadge(copilot edits) = %q, want [edits]", got)
	}

	m.detail = true
	var agentLine string
	for _, line := range strings.Split(m.detailView(), "\n") {
		if strings.Contains(line, "Agent:") {
			agentLine = line
		}
	}
	if !strings.Contains(agentLine, "copilot") {
		t.Fatalf("detail pane Agent line = %q, want it to name copilot", agentLine)
	}
}

// R220.4: a host with no copilot on PATH reports it unavailable the way a
// missing codex is, and the dialog never offers a launch that cannot start.
func TestCreateDialogDoesNotOfferCopilotWhenItIsNotOnPath(t *testing.T) {
	m := openCreateDialog(t, config.Settings{AllowYolo: true}, []string{"claude", "codex", "pi", "shell"})
	if contains(m.createAvailableAgentKinds, "copilot") {
		t.Fatalf("Agent field offers %v although copilot is not on PATH", m.createAvailableAgentKinds)
	}
	if row := agentFieldRow(t, m); !strings.HasSuffix(row.help, "; not on PATH: copilot") {
		t.Fatalf("Agent help = %q, want it to end %q", row.help, "; not on PATH: copilot")
	}
	m.createField = 2
	for i := 0; i < 2*len(m.registry().Kinds()); i++ {
		m.cycleCreateField(1)
		if m.createAgent == "copilot" {
			t.Fatalf("cycling the Agent field reached copilot (offered %v)", m.createAvailableAgentKinds)
		}
	}
	// The mirror image: codex missing, copilot present -> the same sentence for codex.
	m = openCreateDialog(t, config.Settings{}, []string{"claude", "copilot", "pi", "shell"})
	if row := agentFieldRow(t, m); !strings.HasSuffix(row.help, "; not on PATH: codex") {
		t.Fatalf("Agent help = %q, want it to end %q", row.help, "; not on PATH: codex")
	}
}

// R220 (unchanged half): registering copilot changes nothing for the kinds that
// were already there -- their offered profiles, badge, detail Agent line and
// not-on-PATH wording are pinned here as literals.
func TestExistingKindsCreateDialogBadgeAndAvailabilityAreUnchanged(t *testing.T) {
	m := openCreateDialog(t, config.Settings{AllowYolo: true}, []string{"claude", "codex", "copilot", "pi", "shell"})
	wantProfiles := map[string][]string{
		"claude": {"safe", "plan", "edits", "yolo"},
		"codex":  {"safe", "edits", "yolo"},
		"pi":     {"safe", "edits", "yolo"},
		"shell":  {"safe"},
	}
	for kind, want := range wantProfiles {
		if got := m.createProfileOptionsFor(kind, true); !reflect.DeepEqual(got, want) {
			t.Errorf("createProfileOptionsFor(%s) = %v, want %v", kind, got, want)
		}
		picked := cycleAgentTo(t, m, kind)
		if !strings.Contains(picked.createView(), "Agent: "+kind) {
			t.Errorf("create dialog does not show %q", "Agent: "+kind)
		}
		if got := profilesByCycling(picked); !reflect.DeepEqual(got, want) {
			t.Errorf("Permission profile field on %s cycles %v, want %v", kind, got, want)
		}

		session := store.Session{ID: "s-" + kind, Name: "row-" + kind, Slug: "row-" + kind, Agent: kind,
			CWD: "/repo", Status: "running", PermissionProfile: want[len(want)-1], ResumeState: "auto"}
		m.sessions = []store.Session{session}
		m.selected = rowCursor(0)
		badge := m.profileBadge(session)
		if kind == "shell" {
			if badge != "" {
				t.Errorf("shell profile badge = %q, want none", badge)
			}
		} else if badge != "["+want[len(want)-1]+"]" {
			t.Errorf("%s profile badge = %q, want %q", kind, badge, "["+want[len(want)-1]+"]")
		}
		m.detail = true
		if !strings.Contains(m.detailView(), "Agent:              "+kind) {
			t.Errorf("detail pane does not carry the Agent line for %s:\n%s", kind, m.detailView())
		}
		m.detail = false
	}

	// Availability wording for the older kinds is the same sentence as before.
	only := openCreateDialog(t, config.Settings{}, []string{"shell"})
	if row := agentFieldRow(t, only); row.help != "which coding agent adapter launches this session; not on PATH: claude, codex, copilot, pi" {
		t.Fatalf("nothing installed: Agent help = %q", row.help)
	}
	if got := only.createAvailableAgentKinds; !reflect.DeepEqual(got, []string{"shell"}) {
		t.Fatalf("nothing installed: Agent field offers %v, want [shell]", got)
	}
}
