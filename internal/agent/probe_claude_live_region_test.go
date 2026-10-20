package agent

import (
	"strings"
	"testing"
)

// claudeComposer is the composer chrome Claude draws at the bottom of every
// pane: a separator, the (empty) prompt line, a separator and the footer.
var claudeComposer = strings.Join([]string{
	strings.Repeat("─", 60),
	"❯ ",
	strings.Repeat("─", 60),
	"  ⏵⏵ bypass permissions on (shift+tab to cycle)",
}, "\n") + "\n"

// TestClaudeMarkersCountOnlyInTheLiveRegion pins R311: `API Error:` and the
// permission-dialog markers are read only below the last transcript line and
// above the composer. The two "old" cases carry the markers higher up the
// capture, followed by newer transcript output and a live composer.
func TestClaudeMarkersCountOnlyInTheLiveRegion(t *testing.T) {
	cases := []struct {
		name, pane, status, reason string
	}{
		{
			name: "an old API Error above a live composer is not error",
			pane: "● Bash(go test ./...)\n  ⎿  API Error: overloaded_error\n\n" +
				"❯ try again\n\n● Tests pass.\n\n" + claudeComposer,
			status: "idle", reason: "idle footer",
		},
		{
			name: "an answered Do you want to proceed? above a live composer is not waiting",
			pane: "● Bash(go test ./...)\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n" +
				"● Ran go test ./...\n  ⎿  ok  deck 0.4s\n\n● Tests pass.\n\n" + claudeComposer,
			status: "idle", reason: "idle footer",
		},
		{
			name: "a live dialog is waiting",
			pane: "❯ run the tests\n\n● Bash(go test ./...)\n\n Do you want to proceed?\n" +
				" ❯ 1. Yes\n   2. Yes, and don't ask again\n   3. No\n",
			status: "waiting", reason: "permission prompt",
		},
		{
			name: "a live error is error",
			pane: "❯ run the tests\n\n● Bash(go test ./...)\n  ⎿  API Error: overloaded_error\n\n" +
				claudeComposer,
			status: "error", reason: "api error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := NewClaude().Probe(tc.pane)
			if status != tc.status || reason != tc.reason {
				t.Fatalf("Probe = (%q, %q), want (%q, %q)\npane:\n%s", status, reason, tc.status, tc.reason, tc.pane)
			}
		})
	}
}

// TestClaudeLiveRegionBoundaries: the dialog's own numbered cursor is not a
// transcript line, the composer is excluded, and a pane with no transcript
// line is live throughout.
func TestClaudeLiveRegionBoundaries(t *testing.T) {
	dialog := "● Bash(ls)\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n"
	if got := claudeLiveRegion(dialog); !strings.Contains(got, "Do you want to proceed?") {
		t.Fatalf("live region of a live dialog = %q, want the dialog inside it", got)
	}
	if got := claudeLiveRegion(dialog + "\n" + claudeComposer); strings.Contains(got, "bypass permissions") || strings.Contains(got, "❯ \n") {
		t.Fatalf("live region = %q, want the composer excluded", got)
	}
	if got := claudeLiveRegion("API Error: x\nmore\n"); !strings.Contains(got, "API Error: x") {
		t.Fatalf("a pane with no transcript line must be live throughout, got %q", got)
	}
}
