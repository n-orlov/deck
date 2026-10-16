package agent

import (
	"strings"
	"testing"
)

// dividerTails are transcript blocks that open with a line drawn exactly like
// the composer frame (a rule a tool printed, a markdown rule) followed by what
// each agent really draws under a transcript line: claude's flush-left "●"
// bullet, copilot's " ●" and pi's one-column pad, with and without an echoed
// user turn between.
var dividerTails = map[string]map[string][]string{
	"claude": {
		"divider-then-answer":    {"@", "● after divider"},
		"divider-echo-answer":    {"@", "❯ an earlier question", "", "● after divider"},
		"two-dividers":           {"@", "● first", "", "@", "● after divider"},
		"divider-indented-block": {"@", "● after divider", "  ⎿  detail"},
	},
	"copilot": {
		"divider-then-answer": {"@", " ● after divider"},
		"divider-echo-answer": {"@", "❯ an earlier question", "", " ● after divider"},
		"two-dividers":        {"@", " ● first", "", "@", " ● after divider"},
	},
	"pi": {
		"divider-then-text":  {"@", " after divider"},
		"divider-blank-text": {"@", "", " after divider"},
		"two-dividers":       {"@", " first", "", "@", " after divider"},
	},
}

// withTranscriptTail appends tail to golden's transcript, right above its
// composer frame. Each "@" in tail is the golden's own frame rule.
func withTranscriptTail(t *testing.T, golden string, tail []string) string {
	t.Helper()
	frame := frameRuleOf(t, golden)
	lines := strings.Split(golden, "\n")
	top := -1
	for i, n := len(lines)-1, 0; i >= 0; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			if n++; n == 2 {
				top = i
				break
			}
		}
	}
	if top < 0 {
		t.Fatal("golden has no composer frame")
	}
	body := make([]string, 0, len(tail)+1)
	for _, line := range tail {
		if line == "@" {
			line = frame
		}
		body = append(body, line)
	}
	body = append(body, "")
	out := append(append(append([]string{}, lines[:top]...), body...), lines[top:]...)
	return strings.Join(out, "\n")
}

func kindOfGolden(golden string) string {
	return strings.SplitN(golden, "/", 2)[0]
}

// R304/R324/R316: a transcript that holds a line drawn exactly like the composer
// frame is transcript, on the very first capture too. The cut stays at the
// composer's real top, so the text under the divider is in the region (a change
// to it counts), and a draft typed under the real top, even one that mimics the
// frame, still stays out of it.
func TestFrameIdenticalTranscriptDividerIsNotTheComposerTop(t *testing.T) {
	for _, sh := range framedShapes {
		golden := probeGolden(t, sh.golden)
		for name, tail := range dividerTails[kindOfGolden(sh.golden)] {
			t.Run(sh.golden+"/"+name, func(t *testing.T) {
				pane := withTranscriptTail(t, golden, tail)
				cuts := TranscriptCuts(pane)
				if len(cuts) != 1 {
					t.Fatalf("the transcript divider is a second possible composer top: %d cuts", len(cuts))
				}
				if !strings.Contains(cuts[0].Region, "after divider") {
					t.Fatalf("the text under the divider fell out of the region:\n%s", cuts[0].Region)
				}
				changed := strings.Replace(pane, "after divider", "after divider (changed)", 1)
				if TranscriptRegion(changed) == cuts[0].Region {
					t.Error("a real change under the divider did not alter the region")
				}
				if got := lastContentLine(pane); got == lastContentLine(golden) {
					t.Errorf("lastContentLine did not see the transcript tail: %q", got)
				}
				frame := frameRuleOf(t, golden)
				for _, draft := range [][]string{
					{"write an outline"},
					{"write an outline", frame, "keep this unsent"},
					{"a", "", frame, "", frame, "c"},
				} {
					typed := withFramedDraft(t, pane, sh.prompt, draft)
					if got := TranscriptRegion(typed); got != cuts[0].Region {
						t.Errorf("draft %q leaked into the region:\n%s", draft, got)
					}
					edited := append([]string{"edited " + draft[0]}, draft[1:]...)
					if got := TranscriptRegion(withFramedDraft(t, pane, sh.prompt, edited)); got != cuts[0].Region {
						t.Errorf("editing draft %q altered the region:\n%s", draft, got)
					}
				}
			})
		}
	}
}

// The same pane, read before and after a divider arrives: the region changes
// (the divider and what follows it are new transcript), whichever rule cut it.
func TestFrameIdenticalDividerArrivingChangesTheRegion(t *testing.T) {
	for _, sh := range framedShapes {
		golden := probeGolden(t, sh.golden)
		base := TranscriptRegion(golden)
		for name, tail := range dividerTails[kindOfGolden(sh.golden)] {
			pane := withTranscriptTail(t, golden, tail)
			for _, cut := range TranscriptCuts(pane) {
				if cut.Region == base {
					t.Errorf("%s/%s: a cut of the pane with the divider equals the pane without it, so the new output would not count", sh.golden, name)
				}
			}
		}
	}
}
