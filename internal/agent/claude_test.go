package agent

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

func TestClaude_Capabilities(t *testing.T) {
	caps := Claude{}.Capabilities()
	if !caps.AssignsConversationID {
		t.Fatalf("claude Caps.AssignsConversationID = false, want true")
	}
	if !caps.Resumable {
		t.Fatalf("claude Caps.Resumable = false, want true")
	}
	for _, profile := range []string{"safe", "plan", "edits", "yolo"} {
		if !caps.SupportsProfile(profile) {
			t.Fatalf("claude Caps.SupportsProfile(%q) = false, want true", profile)
		}
	}
}

func TestClaude_LaunchAndResumeArgv(t *testing.T) {
	ids := config.NewIDGenerator("claude-adapter-test")
	uuid, err := ids.UUID()
	if err != nil {
		t.Fatalf("UUID: %v", err)
	}

	cases := []struct {
		profile  string
		wantFlag string
	}{
		{"safe", "manual"},
		{"plan", "plan"},
		{"edits", "acceptEdits"},
		{"yolo", "bypassPermissions"},
	}

	claude := Claude{}
	for _, tc := range cases {
		t.Run(tc.profile+"/launch", func(t *testing.T) {
			argv, err := claude.Launch(LaunchInput{
				CWD:            "/tmp/proj",
				ConversationID: uuid,
				Profile:        tc.profile,
			})
			if err != nil {
				t.Fatalf("Launch: %v", err)
			}
			assertContainsPair(t, argv, "--session-id", uuid)
			assertContainsPair(t, argv, "--permission-mode", tc.wantFlag)
			assertNoContinueOrResume(t, argv)
		})
		t.Run(tc.profile+"/resume", func(t *testing.T) {
			argv, err := claude.Resume(ResumeInput{
				CWD:            "/tmp/proj",
				ConversationID: uuid,
				Profile:        tc.profile,
			})
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			assertContainsPair(t, argv, "--resume", uuid)
			assertContainsPair(t, argv, "--permission-mode", tc.wantFlag)
			assertNoContinueOrResume(t, argv)
			for _, a := range argv {
				if a == "--session-id" {
					t.Fatalf("resume argv unexpectedly contains --session-id: %v", argv)
				}
			}
		})
	}
}

func TestClaude_LaunchRequiresConversationID(t *testing.T) {
	if _, err := (Claude{}).Launch(LaunchInput{Profile: "safe"}); err == nil {
		t.Fatalf("Launch with empty ConversationID: want error, got nil")
	}
}

func TestClaude_UnsupportedProfileErrors(t *testing.T) {
	if _, err := (Claude{}).Launch(LaunchInput{ConversationID: "x", Profile: "nonsense"}); err == nil {
		t.Fatalf("Launch with unsupported profile: want error, got nil")
	}
	if _, err := (Claude{}).Resume(ResumeInput{ConversationID: "x", Profile: "nonsense"}); err == nil {
		t.Fatalf("Resume with unsupported profile: want error, got nil")
	}
}

// assertContainsPair fails the test unless argv contains flag immediately
// followed by value.
func assertContainsPair(t *testing.T, argv []string, flag, value string) {
	t.Helper()
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return
		}
	}
	t.Fatalf("argv %v does not contain %q %q", argv, flag, value)
}

// assertNoContinueOrResume fails the test if argv contains any banned
// "most recent conversation" form (SPEC R2): --continue, resume --last, or
// any argument containing the substring "last".
func assertNoContinueOrResume(t *testing.T, argv []string) {
	t.Helper()
	for _, a := range argv {
		if a == "--continue" {
			t.Fatalf("argv %v unexpectedly contains banned --continue", argv)
		}
		if strings.Contains(strings.ToLower(a), "last") {
			t.Fatalf("argv %v unexpectedly contains a 'most recent' form: %q", argv, a)
		}
		if strings.Contains(a, "--dangerously-") {
			t.Fatalf("argv %v unexpectedly contains a --dangerously-* flag: %q", argv, a)
		}
	}
}
