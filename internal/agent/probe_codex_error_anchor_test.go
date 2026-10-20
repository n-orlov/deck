package agent

import "testing"

const codexComposerTail = "\n› Ask Codex to do anything\n\n  local-model low · /tmp/work · Context 0% used\n"

// TestCodexErrorGlyphCountsOnlyAtTheLastContentBlock pins R313: codex's "■"
// error rule is anchored to the last content block above the composer, the way
// pi's "Error:" is anchored to its last content line.
func TestCodexErrorGlyphCountsOnlyAtTheLastContentBlock(t *testing.T) {
	cases := []struct {
		name, pane, status, reason string
	}{
		{
			name: "a ■ in a markdown body is not error",
			pane: "› list the options\n\n• The options are:\n  ■ first option\n  ■ second option\n\n" +
				"  Pick one of them.\n" + codexComposerTail,
			status: "idle", reason: "turn complete",
		},
		{
			name: "a ■ in a progress bar is not error",
			pane: "› download it\n\n• Downloading the archive\n  [■■■■□□□□□□] 40%\n" +
				"\n◦ Working (3s • esc to interrupt)\n" + codexComposerTail,
			status: "running", reason: "working indicator",
		},
		{
			name: "a ■ in tool output followed by more transcript is not error",
			pane: "› run the linter\n\n• Ran golint ./...\n  └ ■ unused variable x\n" +
				"\n■ unexpected status 401 Unauthorized\n\n• The linter finished with one warning.\n" + codexComposerTail,
			status: "idle", reason: "turn complete",
		},
		{
			name: "a trailing ■ error line is error",
			pane: "› hello\n■ unexpected status 401 Unauthorized: {\"detail\":\"Invalid bearer token\"}\n" +
				codexComposerTail,
			status: "error", reason: "terminal error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := NewCodex().Probe(tc.pane)
			if status != tc.status || reason != tc.reason {
				t.Fatalf("Probe = (%q, %q), want (%q, %q)\npane:\n%s", status, reason, tc.status, tc.reason, tc.pane)
			}
		})
	}
}
