package agent

import (
	"strings"
	"testing"
)

// framedDrafts are composer contents a user can really type into a composer
// framed by two separator rules (claude, pi, copilot), each holding a line that
// is itself a rule: a markdown "---" between draft lines, a draft ending in
// one, an ASCII rule at column 0 (pi does not indent continuations), two rules
// in one draft, and a box-drawing rule shorter than the frame.
var framedDrafts = map[string][]string{
	"markdown-rule":   {"write a markdown summary", "  ---", "  keep this draft unsent"},
	"trailing-rule":   {"the draft ends in a rule", "  ---"},
	"column-0-rule":   {"notes", "---", "more notes"},
	"two-rules":       {"a", "  ---", "b", "  -----", "c"},
	"box-rule":        {"a box rule", "  ────────────"},
	"long-ascii-rule": {"", strings.Repeat("-", 60)},
}

type framedShape struct {
	golden, prompt, transcript string
}

var framedShapes = []framedShape{
	{"claude/idle-draft.txt", "❯ ", "Ran 14 tests, all passed."},
	{"claude/running-footer.txt", "❯ ", "I'll refactor the parser next."},
	{"pi/idle.txt", "", "Ten."},
	{"pi/running.txt", "", "Working..."},
	{"copilot/idle-draft.txt", "❯ ", "MCP Servers reloaded"},
}

// withFramedDraft replaces the composer between golden's two frame rules with
// draft (its first line led by prompt). The frame is found from the golden as
// shipped, so a broken boundary cannot move where the draft goes.
func withFramedDraft(t *testing.T, golden, prompt string, draft []string) string {
	t.Helper()
	lines := strings.Split(golden, "\n")
	top, bottom := -1, -1
	for i := len(lines) - 1; i >= 0; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			if bottom < 0 {
				bottom = i
			} else {
				top = i
				break
			}
		}
	}
	if top < 0 {
		t.Fatal("golden has no composer frame")
	}
	body := append([]string{}, draft...)
	body[0] = prompt + body[0]
	out := append(append(append([]string{}, lines[:top+1]...), body...), lines[bottom:]...)
	return strings.Join(out, "\n")
}

// footerOf is the golden's last non-blank line: the status/footer under the
// composer frame.
func footerOf(pane string) string {
	lines := strings.Split(pane, "\n")
	end := trimTrailingBlank(lines, len(lines))
	return lines[end-1]
}

// R304: a framed composer's draft is composer content even when it holds a
// rule line, so neither the draft, an edit to its first line, nor footer churn
// alters the transcript region; a real transcript change still does.
func TestFramedComposerDraftWithRuleLinesIsNotTranscript(t *testing.T) {
	for _, sh := range framedShapes {
		golden := probeGolden(t, sh.golden)
		if !strings.Contains(golden, sh.transcript) {
			t.Fatalf("%s has no %q", sh.golden, sh.transcript)
		}
		base := TranscriptRegion(golden)
		baseLast := lastContentLine(golden)
		for name, draft := range framedDrafts {
			t.Run(sh.golden+"/"+name, func(t *testing.T) {
				pane := withFramedDraft(t, golden, sh.prompt, draft)
				region := TranscriptRegion(pane)
				if region != base {
					t.Fatalf("the draft leaked into the region:\n%s", region)
				}
				if got := lastContentLine(pane); got != baseLast {
					t.Errorf("lastContentLine read the draft: %q, want %q", got, baseLast)
				}
				edited := append([]string{"edited " + draft[0]}, draft[1:]...)
				if got := TranscriptRegion(withFramedDraft(t, golden, sh.prompt, edited)); got != region {
					t.Errorf("editing the draft's first line altered the region:\n%s", got)
				}
				footer := footerOf(pane)
				for _, f := range []string{footer + " ctx:42%", "other-model · /tmp/elsewhere"} {
					if got := TranscriptRegion(strings.Replace(pane, footer, f, 1)); got != region {
						t.Errorf("footer %q altered the region:\n%s", f, got)
					}
				}
				if TranscriptRegion(strings.Replace(pane, sh.transcript, sh.transcript+" (changed)", 1)) == region {
					t.Error("a real transcript change did not alter the region")
				}
			})
		}
	}
}

// The claude live region (R311) cuts at the same frame, so a draft holding a
// rule never reaches the probe: a draft that quotes a waiting marker under an
// idle pane does not read as waiting.
func TestClaudeLiveRegionExcludesDraftWithRuleLine(t *testing.T) {
	golden := probeGolden(t, "claude/idle-draft.txt")
	pane := withFramedDraft(t, golden, "❯ ", []string{"ask it", "  Do you want to proceed?", "  ---", "  then stop"})
	if strings.Contains(claudeLiveRegion(pane), "Do you want to proceed?") {
		t.Fatalf("the draft reached the live region:\n%s", claudeLiveRegion(pane))
	}
}
