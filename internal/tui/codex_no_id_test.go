package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// codexRegistry is a registry carrying the real Codex adapter (never a
// throwaway stand-in) plus shell, for the tests below (PRD R125): the
// window between launch and codex's first prompt, when the row has no
// conversation id yet because codex mints its own on the agent's first
// prompt, never at launch (SPEC §8.2).
func codexRegistry() *agent.Registry {
	r := agent.NewRegistry()
	r.Register(agent.NewShell())
	r.Register(agent.NewCodex())
	return r
}

// buildCodexNoIDModel wires a Model around codexRegistry with session
// already loaded and resume/restart recorded rather than left nil, so a
// refusal below is provably a refusal (the service function was never
// called) and not an accident of Update's own nil-function early return.
func buildCodexNoIDModel(session store.Session) (m Model, resumeCalled, restartCalled *bool) {
	resumeCalled = new(bool)
	restartCalled = new(bool)
	m = New(nil, config.Settings{}, "")
	m.agents = codexRegistry()
	m.resume = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		*resumeCalled = true
		return store.Session{}, service.ResumeStarted, nil
	}
	m.restart = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		*restartCalled = true
		return store.Session{}, service.ResumeStarted, nil
	}
	m.sessions = []store.Session{session}
	m.selected = 0
	return m, resumeCalled, restartCalled
}

// TestResumeRefusesACodexRowWithNoConversationIDYet proves R125's second
// bullet for `r`: a stopped codex row that never got a conversation id
// (killed before its first prompt ever fired SessionStart) refuses with a
// human-worded reason, in m.attachError -- the exact place every other `r`
// refusal already renders through (see TestResumeAlreadyRunningIsNotAnError
// et al.) -- and never calls the wired resume function at all: there is
// nothing on this row for an actual resume attempt to reach, let alone an
// empty id, a guess, or a "most recent" scan (R2).
func TestResumeRefusesACodexRowWithNoConversationIDYet(t *testing.T) {
	session := store.Session{ID: "s1", Name: "codex-no-id", Agent: "codex", Status: "stopped", ConversationID: ""}
	m, resumeCalled, _ := buildCodexNoIDModel(session)

	updated, cmd := m.Update(key("r"))
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("r on an id-less codex row dispatched a command instead of refusing inline")
	}
	if *resumeCalled {
		t.Fatal("r on an id-less codex row called the wired resume function")
	}
	if m.attachError == "" {
		t.Fatal("r on an id-less codex row left no refusal message")
	}
	if !strings.Contains(m.attachError, "Cannot resume") {
		t.Fatalf("attachError = %q, want a Cannot resume refusal", m.attachError)
	}
	if !strings.Contains(m.attachError, "codex") {
		t.Fatalf("attachError = %q, want it to name the agent kind", m.attachError)
	}
	if strings.Contains(m.attachError, "unknown/rejected") {
		t.Fatalf("attachError = %q reads like an adapter launch failure, not a named state", m.attachError)
	}
}

// TestRestartRefusesALiveCodexRowWithNoConversationIDYet proves R125's
// second bullet for `R`: a still-live codex row (canRestart's own
// eligibility, status != stopped) that has not yet had a conversation id
// adopted refuses the same way `r` does, before Restart ever kills the
// live pane or calls Resume underneath it.
func TestRestartRefusesALiveCodexRowWithNoConversationIDYet(t *testing.T) {
	session := store.Session{ID: "s1", Name: "codex-no-id", Agent: "codex", Status: "running", ConversationID: ""}
	m, _, restartCalled := buildCodexNoIDModel(session)

	updated, cmd := m.Update(key("R"))
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("R on an id-less codex row dispatched a command instead of refusing inline")
	}
	if *restartCalled {
		t.Fatal("R on an id-less codex row called the wired restart function")
	}
	if m.attachError == "" {
		t.Fatal("R on an id-less codex row left no refusal message")
	}
	if !strings.Contains(m.attachError, "Cannot restart") {
		t.Fatalf("attachError = %q, want a Cannot restart refusal", m.attachError)
	}
}

// TestResumeAndRestartStillWorkOnceCodexHasAConversationID proves the
// refusal above is scoped to the missing-id window, not to codex as a
// kind: the same adapter with a conversation id already adopted resumes
// exactly like any other adapter would.
func TestResumeAndRestartStillWorkOnceCodexHasAConversationID(t *testing.T) {
	session := store.Session{ID: "s1", Name: "codex-with-id", Agent: "codex", Status: "stopped", ConversationID: "conv-1"}
	m, resumeCalled, _ := buildCodexNoIDModel(session)

	updated, cmd := m.Update(key("r"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("r on a codex row that already has a conversation id did not dispatch a command")
	}
	cmd()
	if !*resumeCalled {
		t.Fatal("r on a codex row that already has a conversation id did not call the wired resume function")
	}
	if m.attachError != "" {
		t.Fatalf("attachError = %q, want no refusal", m.attachError)
	}
}

// TestDetailDialogShowsMissingCodexConversationIDHonestly proves R125's
// third bullet: the `i` detail dialog names the missing id as a state
// ("none yet ...") rather than silently omitting the field the way it
// would for a kind conversation ids do not apply to at all (shell).
func TestDetailDialogShowsMissingCodexConversationIDHonestly(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.agents = codexRegistry()
	session := store.Session{ID: "s1", Name: "codex-no-id", Agent: "codex", Status: "running", ConversationID: ""}
	m.sessions = []store.Session{session}
	m.selected = 0

	body := m.detailBody()
	if !strings.Contains(body, "Conversation id:") {
		t.Fatalf("detail body has no Conversation id field at all for an id-less codex row:\n%s", body)
	}
	if !strings.Contains(body, "none yet") {
		t.Fatalf("detail body did not name the missing id as a state:\n%s", body)
	}

	shellSession := store.Session{ID: "s2", Name: "a-shell", Agent: "shell", Status: "running", ConversationID: ""}
	m.sessions = []store.Session{shellSession}
	m.selected = 0
	shellBody := m.detailBody()
	if strings.Contains(shellBody, "Conversation id:") {
		t.Fatalf("detail body invented a Conversation id field for shell, which has none:\n%s", shellBody)
	}
}

// TestStatusSourceQualityReadsSampledForAnIDlessCodexRowWithNoPerKindBadge
// proves R125's first bullet: a codex row's probe-sourced verdict already
// reads "sampled" through the existing, kind-agnostic statusSourceQuality
// path -- no new per-kind badge, no codex-specific text anywhere in the
// rendered line.
func TestStatusSourceQualityReadsSampledForAnIDlessCodexRowWithNoPerKindBadge(t *testing.T) {
	if got := statusSourceQuality("probe"); got != "sampled" {
		t.Fatalf("statusSourceQuality(probe) = %q, want sampled", got)
	}

	m := New(nil, config.Settings{}, "")
	m.agents = codexRegistry()
	session := store.Session{
		ID: "s1", Name: "codex-no-id", Agent: "codex", Status: "running",
		ConversationID: "", StatusSource: "probe", StatusAt: 1,
	}
	m.sessions = []store.Session{session}
	m.selected = 0

	body := m.detailBody()
	if !strings.Contains(body, "Verdict source:     probe (sampled)") {
		t.Fatalf("detail body did not render the plain probe/sampled verdict source line:\n%s", body)
	}
	if strings.Contains(body, "codex (sampled)") || strings.Contains(body, "codex sampled") {
		t.Fatalf("detail body carries a codex-specific badge instead of the shared quality word:\n%s", body)
	}
}
