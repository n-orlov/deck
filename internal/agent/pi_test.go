package agent

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

func TestPi_Capabilities(t *testing.T) {
	caps := Pi{}.Capabilities()
	if !caps.AssignsConversationID {
		t.Fatalf("pi Caps.AssignsConversationID = false, want true (caller-assigned --session-id)")
	}
	if !caps.Resumable {
		t.Fatalf("pi Caps.Resumable = false, want true")
	}
	for _, profile := range []string{"safe", "edits", "yolo"} {
		if !caps.SupportsProfile(profile) {
			t.Fatalf("pi Caps.SupportsProfile(%q) = false, want true", profile)
		}
	}
	if caps.SupportsProfile("plan") {
		t.Fatalf("pi Caps.SupportsProfile(%q) = true, want false — pi has no plan mode", "plan")
	}
}

func TestPi_LaunchAndResumeUseSessionID(t *testing.T) {
	ids := config.NewIDGenerator("pi-adapter-test")
	uuid, err := ids.UUID()
	if err != nil {
		t.Fatalf("UUID: %v", err)
	}

	pi := Pi{}
	cases := []struct {
		profile      string
		wantApproved bool
	}{
		{"safe", false},
		{"edits", true},
		{"yolo", true},
	}

	for _, tc := range cases {
		t.Run(tc.profile+"/launch", func(t *testing.T) {
			argv, err := pi.Launch(LaunchInput{CWD: "/tmp/proj", ConversationID: uuid, Profile: tc.profile})
			if err != nil {
				t.Fatalf("Launch: %v", err)
			}
			assertContainsPair(t, argv, "--session-id", uuid)
			assertNoContinueOrResume(t, argv)
			assertHasApprove(t, argv, tc.wantApproved)
		})
		t.Run(tc.profile+"/resume", func(t *testing.T) {
			argv, err := pi.Resume(ResumeInput{CWD: "/tmp/proj", ConversationID: uuid, Profile: tc.profile})
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			// Resume uses --session-id too, per SPEC §8: pi has no
			// separate --resume flag; the same flag both creates and
			// resumes a conversation by id.
			assertContainsPair(t, argv, "--session-id", uuid)
			assertNoContinueOrResume(t, argv)
			assertHasApprove(t, argv, tc.wantApproved)
		})
	}
}

func TestPi_LaunchRequiresConversationID(t *testing.T) {
	if _, err := (Pi{}).Launch(LaunchInput{Profile: "safe"}); err == nil {
		t.Fatalf("Launch with empty ConversationID: want error, got nil")
	}
}

func TestPi_PlanDegradesToSafeWithReason(t *testing.T) {
	caps := Pi{}.Capabilities()
	resolved, degraded, reason := caps.ResolveProfile("pi", "plan")
	if !degraded {
		t.Fatalf("ResolveProfile(pi, plan) degraded = false, want true")
	}
	if resolved != "safe" {
		t.Fatalf("ResolveProfile(pi, plan) resolved = %q, want %q", resolved, "safe")
	}
	if reason == "" {
		t.Fatalf("ResolveProfile(pi, plan) reason is empty, want a human-readable explanation")
	}
}

func TestPi_SupportedProfileDoesNotDegrade(t *testing.T) {
	caps := Pi{}.Capabilities()
	resolved, degraded, reason := caps.ResolveProfile("pi", "edits")
	if degraded {
		t.Fatalf("ResolveProfile(pi, edits) degraded = true, want false")
	}
	if resolved != "edits" {
		t.Fatalf("ResolveProfile(pi, edits) resolved = %q, want %q", resolved, "edits")
	}
	if reason != "" {
		t.Fatalf("ResolveProfile(pi, edits) reason = %q, want empty", reason)
	}
}

// assertHasApprove asserts whether argv contains --approve, matching want.
func assertHasApprove(t *testing.T, argv []string, want bool) {
	t.Helper()
	has := false
	for _, a := range argv {
		if a == "--approve" {
			has = true
		}
	}
	if has != want {
		t.Fatalf("argv %v --approve present = %v, want %v", argv, has, want)
	}
}
