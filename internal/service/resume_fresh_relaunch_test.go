package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

const freshRelaunchConversationID = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

// isolateAgentHome points $HOME at a private directory and clears
// CLAUDE_CONFIG_DIR, so the transcript lookup R207 makes never sees the
// operator's own ~/.claude or an ambient override.
func isolateAgentHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return home
}

// recordClaudeTranscript writes the transcript Claude leaves once a
// conversation has received its first message, at Claude's own path.
func recordClaudeTranscript(t *testing.T, home, cwd, conversationID string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, string(filepath.Separator), "-"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, conversationID+".jsonl"), []byte(`{"message":"hello"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func relaunchRegistry() *agent.Registry {
	registry := agent.NewRegistry()
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewCodex())
	return registry
}

func relaunchArgv(t *testing.T, svc Service, session store.Session) []string {
	t.Helper()
	adapter, ok := relaunchRegistry().Lookup(session.Agent)
	if !ok {
		t.Fatalf("no adapter for %q", session.Agent)
	}
	argv, err := svc.resumeArgv(session, adapter, adapter.Capabilities(), agent.LaunchInput{
		CWD: session.CWD, ConversationID: session.ConversationID, Profile: session.PermissionProfile,
	}, false)
	if err != nil {
		t.Fatalf("resumeArgv: %v", err)
	}
	return argv
}

func claudeRelaunchSession(cwd string) store.Session {
	return store.Session{
		Name: "relaunch", CWD: cwd, Agent: "claude", ConversationID: freshRelaunchConversationID,
		PermissionProfile: "safe", ResumeState: "auto",
	}
}

func wantArgvHead(t *testing.T, argv []string, flag string) {
	t.Helper()
	want := []string{"claude", flag, freshRelaunchConversationID}
	if len(argv) < len(want) || !reflect.DeepEqual(argv[:len(want)], want) {
		t.Fatalf("argv = %#v, want it to start with %#v", argv, want)
	}
	other := "--resume"
	if flag == "--resume" {
		other = "--session-id"
	}
	for _, token := range argv {
		if token == other {
			t.Fatalf("argv = %#v must not contain %s", argv, other)
		}
	}
}

// R207 case 1: Claude, not pinned, home known, no CLAUDE_CONFIG_DIR and no
// transcript at the expected path: the relaunch starts the SAME conversation
// id with --session-id. Fails if the branch is reverted to always Resume.
func TestResumeArgvCase1NoTranscriptRelaunchesSameIDWithSessionID(t *testing.T) {
	isolateAgentHome(t)
	session := claudeRelaunchSession(t.TempDir())
	wantArgvHead(t, relaunchArgv(t, Service{}, session), "--session-id")
}

// R207 case 2: a transcript file exists, so the conversation is resumed.
func TestResumeArgvCase2TranscriptExistsResumes(t *testing.T) {
	home := isolateAgentHome(t)
	session := claudeRelaunchSession(t.TempDir())
	recordClaudeTranscript(t, home, session.CWD, session.ConversationID)
	wantArgvHead(t, relaunchArgv(t, Service{}, session), "--resume")
}

// R207 case 3: a pinned conversation is resumed whatever the transcript
// state of the row's own id.
func TestResumeArgvCase3PinnedResumes(t *testing.T) {
	isolateAgentHome(t)
	session := claudeRelaunchSession(t.TempDir())
	session.ResumeState, session.ResumePin = "pinned", session.ConversationID
	wantArgvHead(t, relaunchArgv(t, Service{}, session), "--resume")
}

// R207 case 4: a CLAUDE_CONFIG_DIR override makes the transcript location
// unknowable, so the conversation is resumed -- whichever layer sets it.
func TestResumeArgvCase4ConfigDirOverrideResumes(t *testing.T) {
	isolateAgentHome(t)
	layers := map[string]func(*testing.T, *Service, *store.Session){
		"session env": func(_ *testing.T, _ *Service, s *store.Session) {
			s.Env = map[string]string{"CLAUDE_CONFIG_DIR": "/elsewhere"}
		},
		"config env": func(_ *testing.T, svc *Service, _ *store.Session) {
			svc.ConfigEnv = map[string]string{"CLAUDE_CONFIG_DIR": "/elsewhere"}
		},
		"process env": func(t *testing.T, _ *Service, _ *store.Session) { t.Setenv("CLAUDE_CONFIG_DIR", "/elsewhere") },
	}
	for name, set := range layers {
		t.Run(name, func(t *testing.T) {
			session := claudeRelaunchSession(t.TempDir())
			var svc Service
			set(t, &svc, &session)
			wantArgvHead(t, relaunchArgv(t, svc, session), "--resume")
		})
	}
}

// R207 case 5: an unknown home directory means the transcript path cannot be
// computed, so the conversation is resumed.
func TestResumeArgvCase5UnknownHomeResumes(t *testing.T) {
	isolateAgentHome(t)
	t.Setenv("HOME", "")
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("this platform resolves a home directory without $HOME")
	}
	wantArgvHead(t, relaunchArgv(t, Service{}, claudeRelaunchSession(t.TempDir())), "--resume")
}

// R207 case 6: Codex and Pi are never relaunched fresh, whatever the
// transcript state: their argv is exactly their adapter's resume argv.
func TestResumeArgvCase6CodexAndPiUnchanged(t *testing.T) {
	isolateAgentHome(t)
	for _, kind := range []string{"codex", "pi"} {
		t.Run(kind, func(t *testing.T) {
			session := claudeRelaunchSession(t.TempDir())
			session.Agent = kind
			adapter, _ := relaunchRegistry().Lookup(kind)
			want, err := adapter.Resume(agent.ResumeInput{CWD: session.CWD, ConversationID: session.ConversationID, Profile: session.PermissionProfile})
			if err != nil {
				t.Fatal(err)
			}
			if got := relaunchArgv(t, Service{}, session); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s relaunch argv = %#v, want the adapter's resume argv %#v", kind, got, want)
			}
		})
	}
}

// A Claude session restarted twice before any message is relaunched on its
// own id both times and ends live, never error: a stub that behaves like the
// real CLI (a --resume of a conversation with no transcript fails at once
// with "No conversation found") proves the relaunch never asks it to.
func TestRestartTwiceBeforeFirstMessageEndsRunningNotError(t *testing.T) {
	isolateAgentHome(t)
	cwd := t.TempDir()
	binDir := t.TempDir()
	stub := "#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = \"--resume\" ]; then echo 'No conversation found' >&2; exit 1; fi\ndone\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	service, db, _, _ := newAgentTestService(t, nil, "restart-twice")
	ctx := context.Background()

	created, err := service.CreateAgent(ctx, AgentCreateInput{Name: "Claude: twice", CWD: cwd, Agent: "claude", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	for restart := 1; restart <= 2; restart++ {
		restarted, outcome, err := service.Restart(ctx, created.ID)
		if err != nil || outcome != ResumeStarted {
			t.Fatalf("restart %d = (%v, %v), want ResumeStarted and no error", restart, outcome, err)
		}
		if restarted.ConversationID != created.ConversationID {
			t.Fatalf("restart %d conversation id = %q, want unchanged %q", restart, restarted.ConversationID, created.ConversationID)
		}
	}

	// The agent's own hook is what promotes a live Claude row to running.
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{SessionID: created.ID, Status: "running", Source: "hook", At: 2, EventKind: "hook.SessionStart"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row, err := db.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "running" {
		t.Fatalf("status after two restarts and reconcile = %q (%s), want running", row.Status, row.StatusReason)
	}
	if live, err := service.TMux.HasLivePane(ctx, created.Slug); err != nil || !live {
		t.Fatalf("live pane = %v, %v, want a live pane after the second restart", live, err)
	}
}

// R207 case 2 (conservative): a transcript lookup that fails for a reason other
// than a confirmed not-exist leaves the state unknown, so the conversation is
// resumed rather than started again on the same id.
func TestResumeArgvLookupFailureResumes(t *testing.T) {
	home := isolateAgentHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// ~/.claude/projects is a regular file, so the lookup fails with ENOTDIR.
	if err := os.WriteFile(filepath.Join(home, ".claude", "projects"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantArgvHead(t, relaunchArgv(t, Service{}, claudeRelaunchSession(t.TempDir())), "--resume")
}

// A dangling symlink at the transcript path is an existing entry whose
// metadata cannot be read: it is resumed, never relaunched fresh.
func TestResumeArgvUnreadableTranscriptEntryResumes(t *testing.T) {
	home := isolateAgentHome(t)
	session := claudeRelaunchSession(t.TempDir())
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(session.CWD, string(filepath.Separator), "-"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(dir, session.ConversationID+".jsonl")); err != nil {
		t.Fatal(err)
	}
	wantArgvHead(t, relaunchArgv(t, Service{}, session), "--resume")
}
