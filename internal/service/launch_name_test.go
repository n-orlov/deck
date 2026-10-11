package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// TestDerivedNoNameLaunchNameFromRow proves LaunchInput.Name is filled from the
// row only when name_source is 'user' (R250): a derived or harness row, or a
// blank name, hands the adapter nothing.
func TestDerivedNoNameLaunchNameFromRow(t *testing.T) {
	for _, tc := range []struct {
		name, source, row, want string
	}{
		{"user", "user", "my work", "my work"},
		{"empty source is user", "", "my work", "my work"},
		{"derived", "derived", "swift-otter", ""},
		{"harness", "harness", "Fix the bug", ""},
		{"blank user name", "user", "  ", ""},
	} {
		got := launchName(store.Session{Name: tc.row, NameSource: tc.source})
		if got != tc.want {
			t.Errorf("%s: launchName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// nameSpyClaude is Claude recording the Name each launch and resume receives.
type nameSpyClaude struct {
	agent.Claude
	launchNames, resumeNames []string
}

func (s *nameSpyClaude) Launch(in agent.LaunchInput) ([]string, error) {
	s.launchNames = append(s.launchNames, in.Name)
	return s.Claude.Launch(in)
}

func (s *nameSpyClaude) Resume(in agent.ResumeInput) ([]string, error) {
	s.resumeNames = append(s.resumeNames, in.Name)
	return s.Claude.Resume(in)
}

// TestDerivedNoNameCreateFillsLaunchInputNameOnlyForUserRows drives the real
// create path with a spy adapter: a user row's name reaches LaunchInput.Name, a
// derived row's does not.
func TestDerivedNoNameCreateFillsLaunchInputNameOnlyForUserRows(t *testing.T) {
	stubExecutableOnPath(t, "claude")
	for _, tc := range []struct{ source, want string }{{"user", "typed name"}, {"derived", ""}} {
		svc, _, _, _ := newAgentTestService(t, nil, "launch-name-"+tc.source)
		spy := &nameSpyClaude{}
		registry := agent.NewRegistry()
		registry.Register(spy)
		svc.Agents = registry
		if _, err := svc.CreateAgent(context.Background(), AgentCreateInput{
			Name: "typed name", CWD: t.TempDir(), Agent: "claude", NameSource: tc.source,
		}); err != nil {
			t.Fatalf("%s create: %v", tc.source, err)
		}
		if len(spy.launchNames) != 1 || spy.launchNames[0] != tc.want {
			t.Errorf("name_source=%s: LaunchInput.Name = %q, want one launch with %q", tc.source, spy.launchNames, tc.want)
		}
	}
}

// TestDerivedNoNameResumeArgvCarriesLaunchInputName proves the resume branch
// hands the adapter the same Name the launch input holds.
func TestDerivedNoNameResumeArgvCarriesLaunchInputName(t *testing.T) {
	svc, _, _, _ := newAgentTestService(t, nil, "launch-name-resume")
	spy := &nameSpyClaude{}
	home := isolateAgentHome(t)
	session := claudeRelaunchSession(t.TempDir())
	// A recorded transcript makes the relaunch a Resume, not a fresh Launch.
	recordClaudeTranscript(t, home, session.CWD, session.ConversationID)
	for _, name := range []string{"typed", ""} {
		if _, err := svc.resumeArgv(session, spy, spy.Capabilities(), agent.LaunchInput{
			CWD: session.CWD, ConversationID: session.ConversationID, Profile: "safe", Name: name,
		}, false); err != nil {
			t.Fatalf("resumeArgv: %v", err)
		}
	}
	if len(spy.resumeNames) != 2 || spy.resumeNames[0] != "typed" || spy.resumeNames[1] != "" {
		t.Fatalf("ResumeInput.Name seen = %q, want [typed \"\"]", spy.resumeNames)
	}
}
