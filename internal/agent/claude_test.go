package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
		wantFlag string // empty means no --permission-mode flag at all (R116)
	}{
		{"safe", ""},
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
			if tc.wantFlag == "" {
				assertNoPermissionModeFlag(t, argv)
			} else {
				assertContainsPair(t, argv, "--permission-mode", tc.wantFlag)
			}
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
			if tc.wantFlag == "" {
				assertNoPermissionModeFlag(t, argv)
			} else {
				assertContainsPair(t, argv, "--permission-mode", tc.wantFlag)
			}
			assertNoContinueOrResume(t, argv)
			for _, a := range argv {
				if a == "--session-id" {
					t.Fatalf("resume argv unexpectedly contains --session-id: %v", argv)
				}
			}
		})
	}
}

func TestClaude_InstrumentReturnsInlineHooksAndDeckEnvironmentWithoutIO(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "session-cwd")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	t.Setenv("HOME", home)

	deckExecutable := "/opt/deck builds/deck's-bin"
	argv, env := (Claude{}).Instrument(LaunchInput{
		CWD:            cwd,
		ConversationID: "claude-conversation",
		DeckExecutable: deckExecutable,
		DeckSessionID:  "deck-row-42",
		DeckHome:       "/tmp/scenario-deck-home",
	})
	if len(argv) != 2 || argv[0] != "--settings" {
		t.Fatalf("Instrument argv = %#v, want one inline --settings value", argv)
	}
	// DECK_SESSION_ID and DECK_HOME moved to internal/service's own
	// session-context map (SPEC §6.1, R104); with no LaunchGeneration set
	// (this call's LaunchInput carries none), Claude's own instrumentation
	// now contributes no environment at all.
	if env != nil {
		t.Fatalf("Instrument env = %#v, want nil (session context and launch generation both absent)", env)
	}

	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(argv[1]), &settings); err != nil {
		t.Fatalf("decode inline settings: %v", err)
	}
	gotEvents := make([]string, 0, len(settings.Hooks))
	for event := range settings.Hooks {
		gotEvents = append(gotEvents, event)
	}
	sort.Strings(gotEvents)
	wantEvents := []string{"Notification", "SessionEnd", "SessionStart", "Stop", "StopFailure", "UserPromptSubmit"}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("instrumented events = %v, want %v", gotEvents, wantEvents)
	}
	wantCommand := `'/opt/deck builds/deck'"'"'s-bin' _hook`
	for _, event := range wantEvents {
		groups := settings.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s hook shape = %#v, want one command hook", event, groups)
		}
		hook := groups[0].Hooks[0]
		if hook.Type != "command" || hook.Command != wantCommand {
			t.Fatalf("%s hook = %#v, want command %q", event, hook, wantCommand)
		}
	}

	for label, directory := range map[string]string{"HOME": home, "session cwd": cwd} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("read %s: %v", label, err)
		}
		if len(entries) != 0 {
			t.Fatalf("Instrument wrote under %s: %v", label, entries)
		}
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

// assertNoPermissionModeFlag fails the test if argv contains a
// --permission-mode flag at all (R116: the "safe" profile composes none).
func assertNoPermissionModeFlag(t *testing.T, argv []string) {
	t.Helper()
	for _, a := range argv {
		if a == "--permission-mode" {
			t.Fatalf("argv %v unexpectedly contains --permission-mode", argv)
		}
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
