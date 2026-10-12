package agent

import "testing"

const claudeGateRule = "────────────────────────────────────────────────────────────\n"

// TestClaudeGateScreens (R267): Claude is a DialogDetector and a PromptGate,
// and only an empty composer with no dialog is ready for the rename plan.
func TestClaudeGateScreens(t *testing.T) {
	permission := " Bash command\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n Esc to cancel\n"
	for _, tc := range []struct {
		name       string
		screen     string
		wantDialog bool
		wantReady  bool
	}{
		{name: "permission dialog", screen: permission, wantDialog: true},
		{name: "trust screen", screen: " Do you trust the files in this folder?\n ❯ 1. Yes, proceed\n", wantDialog: true},
		{name: "empty composer", screen: "● hi\n" + claudeGateRule + "❯\n" + claudeGateRule, wantReady: true},
		{name: "boxed empty composer", screen: "╭──╮\n│ >            │\n╰──╯\n", wantReady: true},
		{name: "placeholder composer", screen: claudeGateRule + "❯ Try \"fix lint errors\"\n" + claudeGateRule, wantReady: true},
		{name: "typed draft", screen: claudeGateRule + "❯ half a prompt\n" + claudeGateRule},
		{name: "echoed turn above an empty composer", screen: "❯ earlier\n● ok\n" + claudeGateRule + "❯\n", wantReady: true},
		{name: "startup with no composer", screen: "Starting Claude Code\n"},
		{name: "blank screen", screen: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Claude{}
			if got := c.DialogFocused(tc.screen); got != tc.wantDialog {
				t.Fatalf("DialogFocused = %v, want %v", got, tc.wantDialog)
			}
			if got := c.PromptReady(tc.screen); got != tc.wantReady {
				t.Fatalf("PromptReady = %v, want %v", got, tc.wantReady)
			}
		})
	}
	var adapter Adapter = NewClaude()
	if !IsDialogDetector(adapter) || !IsPromptGate(adapter) {
		t.Fatal("Claude must implement DialogDetector and PromptGate")
	}
}
