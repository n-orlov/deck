package agent

import (
	"reflect"
	"strings"
	"testing"
)

func copilotNameArgv(t *testing.T, name string) []string {
	t.Helper()
	argv, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: "safe", Name: name})
	if err != nil {
		t.Fatalf("Launch(%q): %v", name, err)
	}
	return argv
}

// TestCopilotNameFirstLaunchCarriesDashN (R259): a first launch with a name
// passes `--name=<name>` exactly once, before --no-auto-update and the launch args.
func TestCopilotNameFirstLaunchCarriesDashN(t *testing.T) {
	argv, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: "edits", Name: "my work", ExtraArgs: []string{"--model", "gpt-5"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"copilot", "--session-id", copilotTestID, "--allow-tool=write", "--name=my work", "--no-auto-update", "--model", "gpt-5"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %q, want %q", argv, want)
	}
}

// TestCopilotNameResumeAndDerivedCarryNoDashN: Resume (a relaunch of an existing
// conversation) never carries -n, even given a name; a blank name (the
// derived/harness row's LaunchInput.Name) never does either.
func TestCopilotNameResumeAndDerivedCarryNoDashN(t *testing.T) {
	resume, err := NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: "safe", Name: "my work"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"copilot", "--session-id", copilotTestID, "--no-auto-update"}
	if !reflect.DeepEqual(resume, want) {
		t.Fatalf("Resume argv = %q, want %q", resume, want)
	}
	for _, blank := range []string{"", "   ", "\t"} {
		if got := copilotNameArgv(t, blank); !reflect.DeepEqual(got, want) {
			t.Errorf("Launch name %q argv = %q, want %q", blank, got, want)
		}
	}
}

// TestCopilotNameSanitiser: `"` becomes `'`, the name is trimmed and cut to
// 100 runes.
func TestCopilotNameSanitiser(t *testing.T) {
	r100 := strings.Repeat("é", 100)
	for _, tc := range []struct{ name, in, want string }{
		{"100 runes kept", r100, r100},
		{"101 runes clamped", r100 + "x", r100},
		{"embedded quote", `say "hi" now`, `say 'hi' now`},
		{"trimmed", "  padded  ", "padded"},
		{"clamp does not leave trailing space", strings.Repeat("a", 99) + " b", strings.Repeat("a", 99)},
	} {
		argv := copilotNameArgv(t, tc.in)
		if i := indexOf(argv, "--name="+tc.want); i < 0 {
			t.Errorf("%s: argv = %q, want --name=%q", tc.name, argv, tc.want)
		}
	}
	if got := len([]rune(copilotLaunchName(r100 + r100))); got != 100 {
		t.Errorf("clamped length = %d, want 100", got)
	}
}

func indexOf(argv []string, s string) int {
	for i, a := range argv {
		if a == s {
			return i
		}
	}
	return -1
}
