package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const copilotNameFixtureID = "0b6f1c1e-5d1a-4a53-9a43-3f1d2b0c7a11"

func copilotNameInput(t *testing.T, fixture string) NameProbeInput {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("testdata", "names", "copilot", fixture))
	if err != nil {
		t.Fatal(err)
	}
	return NameProbeInput{ConversationID: copilotNameFixtureID, Env: map[string]string{"COPILOT_HOME": home}}
}

// TestCopilotReadNameShapes: one fixture per workspace.yaml shape. user_named
// false with a name is the harness's own title; user_named true is explicit; no
// name is none; quoted names round-trip.
func TestCopilotReadNameShapes(t *testing.T) {
	for _, tc := range []struct {
		fixture  string
		name     string
		explicit bool
		ok       bool
	}{
		{"harness-title", "Fix the login bug", false, true},
		{"explicit", "my review", true, true},
		{"no-name", "", false, false},
		{"quoted-colon", "fix: parser # not a comment", true, true},
		{"quoted-apostrophe", `it's the "parser" bug`, false, true},
		{"quoted-unicode", "Überprüfung: 日本語 🚀", true, true},
		{"other-id", "", false, false},
		{"not-yaml", "", false, false},
		{"missing", "", false, false},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			name, explicit, ok := NewCopilot().ReadName(copilotNameInput(t, tc.fixture))
			if name != tc.name || explicit != tc.explicit || ok != tc.ok {
				t.Fatalf("ReadName = (%q, %v, %v), want (%q, %v, %v)", name, explicit, ok, tc.name, tc.explicit, tc.ok)
			}
		})
	}
}

// TestCopilotReadNameDispatchesThroughCaps: the dispatcher asks Copilot (its
// caps claim yaml) and gets the same answer.
func TestCopilotReadNameDispatchesThroughCaps(t *testing.T) {
	name, explicit, ok := ReadName(NewCopilot(), copilotNameInput(t, "explicit"))
	if name != "my review" || !explicit || !ok {
		t.Fatalf("ReadName = (%q, %v, %v)", name, explicit, ok)
	}
}

// TestCopilotReadNameFallsBackToHome: without COPILOT_HOME the file is under
// <Home>/.copilot, and an unsafe conversation id is never joined into a path.
func TestCopilotReadNameFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".copilot", "session-state", copilotNameFixtureID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := "id: " + copilotNameFixtureID + "\nname: from home\nuser_named: false\n"
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	in := NameProbeInput{Home: home, ConversationID: copilotNameFixtureID}
	if name, _, ok := NewCopilot().ReadName(in); !ok || name != "from home" {
		t.Fatalf("ReadName = (%q, %v)", name, ok)
	}
	in.ConversationID = "../" + copilotNameFixtureID
	if _, _, ok := NewCopilot().ReadName(in); ok {
		t.Fatal("an unsafe conversation id was read")
	}
}

// TestCopilotReadNameIsBounded: a workspace.yaml bigger than the read bound
// is read only up to the bound, so a name placed after it is never seen.
func TestCopilotReadNameIsBounded(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "session-state", copilotNameFixtureID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := "id: " + copilotNameFixtureID + "\n# " + strings.Repeat("x", copilotWorkspaceBytes) + "\nname: too far\nuser_named: true\n"
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	in := NameProbeInput{ConversationID: copilotNameFixtureID, Env: map[string]string{"COPILOT_HOME": root}}
	if name, _, ok := NewCopilot().ReadName(in); ok {
		t.Fatalf("read past the bound: %q", name)
	}
}

// TestCopilotPushPlan: /rename <name> then a lone Enter, name sanitised.
func TestCopilotPushPlan(t *testing.T) {
	plan, ok := PushName(NewCopilot(), NamePushInput{Name: "fix the \"login\" bug"})
	if !ok || len(plan) != 2 || plan[0] != "/rename fix the 'login' bug" || plan[1] != "Enter" {
		t.Fatalf("plan = %q, ok=%v", plan, ok)
	}
	if _, ok := NewCopilot().PushName(NamePushInput{Name: " \n "}); ok {
		t.Fatal("a blank name was pushed")
	}
}

// TestCopilotDialogFocused: the folder-trust, permission and question dialogs
// captured from copilot 1.0.93 are focused dialogs a typed rename would land
// in; the idle, draft, working and error screens are not (R260 push gate).
func TestCopilotDialogFocused(t *testing.T) {
	cases := map[string]bool{
		"trust.txt": true, "permission.txt": true, "question.txt": true,
		"idle.txt": false, "idle-draft.txt": false, "working.txt": false,
		"working-edit-prompt.txt": false, "error.txt": false, "warning.txt": false,
	}
	for file, want := range cases {
		screen, err := os.ReadFile(filepath.Join(copilotFixtureDir, file))
		if err != nil {
			t.Fatal(err)
		}
		if got := DialogFocused(NewCopilot(), string(screen)); got != want {
			t.Errorf("%s: DialogFocused = %v, want %v", file, got, want)
		}
	}
	if !IsDialogDetector(NewCopilot()) {
		t.Error("Copilot is not asked about a focused dialog before a push")
	}
}
