package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// callRecordingCopilot wraps the real Copilot adapter and records which of
// Launch and Resume the service asked for. Embedding promotes every method of
// agent.Copilot, so it would also promote RelaunchFresh if Copilot ever grew
// one -- which is exactly what the test below must notice.
type callRecordingCopilot struct {
	agent.Copilot
	launches, resumes int
}

func (c *callRecordingCopilot) Launch(in agent.LaunchInput) ([]string, error) {
	c.launches++
	return c.Copilot.Launch(in)
}

func (c *callRecordingCopilot) Resume(in agent.ResumeInput) ([]string, error) {
	c.resumes++
	return c.Copilot.Resume(in)
}

// R216: a copilot session's relaunch takes the Resume path, never the
// fresh-relaunch path, whether or not a transcript exists (`--session-id`
// resumes a session killed before its first message, so no fresh relaunch is
// needed), and a pinned row is no different.
func TestCopilotRelaunchTakesResumePathNeverFreshRelaunch(t *testing.T) {
	home := isolateAgentHome(t)
	t.Setenv("COPILOT_HOME", "")
	for name, haveTranscript := range map[string]bool{"no transcript yet": false, "transcript recorded": true} {
		for _, resumeState := range []string{"auto", "pinned"} {
			t.Run(name+"/"+resumeState, func(t *testing.T) {
				session := store.Session{
					Name: "copilot", CWD: t.TempDir(), Agent: "copilot", ConversationID: freshRelaunchConversationID,
					PermissionProfile: "edits", LaunchArgs: []string{"--model", "x"}, ResumeState: resumeState,
				}
				if resumeState == "pinned" {
					session.ResumePin = session.ConversationID
				}
				if haveTranscript {
					recordCopilotTranscript(t, home, session.ConversationID)
				}
				adapter := &callRecordingCopilot{}
				if _, ok := agent.Adapter(adapter).(agent.FreshRelauncher); ok {
					t.Fatal("the copilot adapter implements FreshRelauncher")
				}
				svc := Service{}
				if svc.relaunchFresh(session, adapter, session.ConversationID) {
					t.Fatal("relaunchFresh = true for a copilot session, want false")
				}
				argv, err := svc.resumeArgv(session, adapter, adapter.Capabilities(), agent.LaunchInput{
					CWD: session.CWD, ConversationID: session.ConversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
				}, false)
				if err != nil {
					t.Fatalf("resumeArgv: %v", err)
				}
				if adapter.resumes != 1 || adapter.launches != 0 {
					t.Fatalf("resume path calls: Resume x%d, Launch x%d; want Resume x1 and Launch x0", adapter.resumes, adapter.launches)
				}
				want := []string{"copilot", "--session-id", session.ConversationID, "--allow-tool=write", "--no-auto-update", "--model", "x"}
				if !reflect.DeepEqual(argv, want) {
					t.Fatalf("relaunch argv = %#v, want %#v", argv, want)
				}
			})
		}
	}
}

func recordCopilotTranscript(t *testing.T, home, conversationID string) {
	t.Helper()
	dir := filepath.Join(home, ".copilot", "session-state", conversationID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
