package agent

import (
	"strings"
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

// R204 (#56): Pi's launch installs the deck-owned extension and the hook
// command of the launching deck binary, and carries the launch generation like
// Claude and Codex do (absent without a lease).
func TestPiInstrumentInstallsTheExtensionAndTheLaunchingDeckHookCommand(t *testing.T) {
	in := LaunchInput{DeckExecutable: "/opt/deck/bin/deck", DeckHome: "/data/deck", LaunchGeneration: "gen-7"}
	argv, env := Pi{}.Instrument(in)
	if len(argv) != 2 || argv[0] != "-e" || argv[1] != "/data/deck/pi/deck-hook.js" {
		t.Fatalf("pi Instrument argv = %#v, want -e <data root>/pi/deck-hook.js", argv)
	}
	if env[PiHookCommandEnv] != `'/opt/deck/bin/deck' _hook` || env[LaunchGenerationEnv] != "gen-7" || len(env) != 2 {
		t.Fatalf("pi Instrument env = %#v", env)
	}
	in.LaunchGeneration = ""
	if _, env := (Pi{}).Instrument(in); len(env) != 1 || env[PiHookCommandEnv] == "" {
		t.Fatalf("pi Instrument env without a lease = %#v, want only the hook command", env)
	}
}

func TestPiInstrumentFilesWritesTheExtensionUnderTheDataRootOnly(t *testing.T) {
	files := Pi{}.InstrumentFiles(LaunchInput{DeckHome: "/data/deck"})
	if len(files) != 1 || files[0].Path != PiExtensionPath("/data/deck") || string(files[0].Content) != PiExtensionSource {
		t.Fatalf("pi InstrumentFiles = %#v", files)
	}
	if files := (Pi{}).InstrumentFiles(LaunchInput{}); len(files) != 0 {
		t.Fatalf("pi InstrumentFiles without a data root = %#v, want none", files)
	}
	var _ FileInstrumenter = Pi{}
}

// The extension fires exactly the events PiHookEvents lists, reads the
// command from PiHookCommandEnv, and reports the in-session shutdown reasons
// `_hook` keeps from stopping a row.
func TestPiExtensionSourceSubscribesEveryHookEvent(t *testing.T) {
	for _, want := range append([]string{PiHookCommandEnv, "session_start", "before_agent_start", "agent_settled", "session_shutdown", "getSessionId", `/bin/sh`}, quoted(PiHookEvents)...) {
		if !strings.Contains(PiExtensionSource, want) {
			t.Errorf("pi extension lacks %q", want)
		}
	}
	for _, reason := range []string{"new: \"clear\"", "resume: \"resume\"", "fork: \"resume\"", "reload: \"resume\""} {
		if !strings.Contains(PiExtensionSource, reason) {
			t.Errorf("pi extension lacks the in-session reason %q", reason)
		}
	}
}

func quoted(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = `"` + v + `"`
	}
	return out
}
