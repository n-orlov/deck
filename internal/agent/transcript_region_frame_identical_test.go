package agent

import (
	"strings"
	"testing"
)

// frameRuleOf is the golden's composer frame rule, as drawn.
func frameRuleOf(t *testing.T, golden string) string {
	t.Helper()
	for _, line := range strings.Split(golden, "\n") {
		if strings.TrimSpace(line) != "" && isSeparatorLine(strings.TrimSpace(line)) {
			return line
		}
	}
	t.Fatal("golden has no frame rule")
	return ""
}

// R304/R324/R316: a framed draft can hold a line drawn exactly like the frame's
// own rules (pasted, or typed to the pane's width). Such a line is composer
// content: the cut stays at the frame's top rule, so neither the draft nor an
// edit to it reaches the transcript region, while a real transcript change
// still does.
func TestFrameIdenticalDraftRuleIsNotTheFrame(t *testing.T) {
	for _, sh := range framedShapes {
		golden := probeGolden(t, sh.golden)
		frame := frameRuleOf(t, golden)
		base := TranscriptRegion(golden)
		baseLast := lastContentLine(golden)
		drafts := map[string][]string{
			"rule-between-text":      {"write an outline", frame, "keep this unsent"},
			"rule-after-blank":       {"write an outline", "", frame, "keep this unsent"},
			"rule-last":              {"write an outline", frame},
			"rule-twice":             {"a", frame, "b", frame, "c"},
			"rule-twice-after-blank": {"a", "", frame, "", frame, "c"},
			"rule-with-padding":      {"a", frame + "   ", "b"},
		}
		for name, draft := range drafts {
			t.Run(sh.golden+"/"+name, func(t *testing.T) {
				pane := withFramedDraft(t, golden, sh.prompt, draft)
				if got := TranscriptRegion(pane); got != base {
					t.Fatalf("the draft leaked into the region:\n%s", strings.TrimPrefix(got, base))
				}
				if got := lastContentLine(pane); got != baseLast {
					t.Errorf("lastContentLine = %q, want %q", got, baseLast)
				}
				edited := append([]string{"edited " + draft[0]}, draft[1:]...)
				if got := TranscriptRegion(withFramedDraft(t, golden, sh.prompt, edited)); got != base {
					t.Errorf("editing the draft altered the region:\n%s", got)
				}
				if TranscriptRegion(strings.Replace(pane, sh.transcript, sh.transcript+" (changed)", 1)) == base {
					t.Error("a real transcript change did not alter the region")
				}
			})
		}
	}
}

// The claude live region cuts at the same frame, so a draft quoting a waiting
// marker behind a frame-identical rule does not read as waiting.
func TestClaudeLiveRegionExcludesDraftWithFrameIdenticalRule(t *testing.T) {
	golden := probeGolden(t, "claude/idle-draft.txt")
	frame := frameRuleOf(t, golden)
	pane := withFramedDraft(t, golden, "❯ ", []string{"ask it", "Do you want to proceed?", frame, "then stop"})
	if strings.Contains(claudeLiveRegion(pane), "Do you want to proceed?") {
		t.Fatalf("the draft reached the live region:\n%s", claudeLiveRegion(pane))
	}
}
