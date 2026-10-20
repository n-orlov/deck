package agent

import (
	"strings"
	"testing"
)

// R304: pi pads every transcript line by at least one column, its own text by
// exactly one and a code block or tool output by more. Transcript text under a
// divider drawn exactly like the frame is transcript at any depth, also on the
// first capture and also when the composer below holds a draft (one with a
// frame-identical rule of its own included). "%s" is the value the test edits;
// "@" is the frame rule. The first tail is the shape review found; the others
// are the same rule in shapes it did not list.
var piIndentedTranscriptTails = map[string][]string{
	"finding-3-cols":    {"@", "   code: return verifierValue(%s)"},
	"two-cols":          {"@", "  result: %s"},
	"tab":               {"@", "\tvalue = %s"},
	"code-block":        {"@", "   ```go", "     return verifierValue(%s)", "   ```"},
	"two-deep-dividers": {"@", "    first block", "", "@", "    second block %s"},
}

var piDraftsUnderIndentedTranscript = map[string][]string{
	"empty":          {""},
	"column-0-draft": {"please explain"},
	"mimicking":      {"please explain", "  preserve this indentation", "", "@", "appendix"},
}

func TestPiIndentedTranscriptUnderDividerIsTranscript(t *testing.T) {
	for _, name := range []string{"pi/idle.txt", "pi/running.txt"} {
		golden := probeGolden(t, name)
		frame := frameRuleOf(t, golden)
		for tailName, tail := range piIndentedTranscriptTails {
			for draftName, draft := range piDraftsUnderIndentedTranscript {
				t.Run(name+"/"+tailName+"/"+draftName, func(t *testing.T) {
					pane := func(value string) string {
						filled := make([]string, len(tail))
						for i, line := range tail {
							filled[i] = strings.Replace(line, "%s", value, 1)
						}
						base := withTranscriptTail(t, golden, filled)
						return withFramedDraft(t, base, "", fillPiDraft(draft, frame, ""))
					}
					first := pane("41")
					region := TranscriptRegion(first)
					if !strings.Contains(region, "41") {
						t.Fatalf("indented transcript under a divider was cut away on first sight:\n%s", region)
					}
					if draftName != "empty" && strings.Contains(region, "please explain") {
						t.Fatalf("the draft leaked into the transcript region:\n%s", region)
					}
					for _, cut := range TranscriptCuts(pane("42")) {
						if cut.Region == region {
							t.Fatal("a change to the indented transcript is hidden by one of the cuts")
						}
					}
				})
			}
		}
	}
}
