package agent

import (
	"strings"
	"testing"
)

// R304/R324/R316: a framed draft can put a line drawn exactly like the frame's
// rule right above a quoted "❯" line, which is what claude's and copilot's own
// composer top looks like. In pi, which draws no prompt glyph, that is draft
// text; in claude and copilot the frame's own "❯" line is above it. Either way
// the cut stays at the frame's top rule: neither the draft nor an edit anywhere
// in it reaches the transcript region (or claude's live region), while a real
// transcript change still does.
func TestQuotedPromptUnderFrameIdenticalDraftRuleIsNotTheFrame(t *testing.T) {
	for _, sh := range framedShapes {
		golden := probeGolden(t, sh.golden)
		frame := frameRuleOf(t, golden)
		base := TranscriptRegion(golden)
		baseLast := lastContentLine(golden)
		drafts := map[string][]string{
			// the reviewer's shape
			"text-blank-rule-prompt-text": {"ordinary draft text", "", frame, "❯ quoted prompt, not an agent turn", "trailing draft text"},
			"blank-first-rule-prompt":     {"", frame, "❯ quoted"},
			"rule-prompt-no-blank":        {"a", frame, "❯ quoted", "b"},
			"rule-prompt-indented-tail":   {"first", "", frame, "❯ quoted", "  indented tail"},
			"two-quoted-prompts":          {"a", "", frame, "❯ one", "", frame, "❯ two"},
			"rule-prompt-last":            {"a", "", frame, "❯ "},
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
				if strings.HasPrefix(sh.golden, "claude/") && strings.Contains(claudeLiveRegion(pane), "❯ quoted") {
					t.Errorf("the draft reached claude's live region:\n%s", claudeLiveRegion(pane))
				}
				for i := range draft {
					edited := append([]string{}, draft...)
					edited[i] = "edited " + edited[i]
					if got := TranscriptRegion(withFramedDraft(t, golden, sh.prompt, edited)); got != base {
						t.Errorf("editing draft line %d altered the region:\n%s", i, strings.TrimPrefix(got, base))
					}
				}
				if TranscriptRegion(strings.Replace(pane, sh.transcript, sh.transcript+" (changed)", 1)) == base {
					t.Error("a real transcript change did not alter the region")
				}
			})
		}
	}
}

// Pi's frame is glyphless (its cwd line under the bottom rule starts at
// column 0); claude's and copilot's are not, so the prompt-glyph evidence is
// read only for them.
func TestGlyphlessFrameIsPiOnly(t *testing.T) {
	for _, name := range []string{"pi/idle.txt", "pi/running.txt", "pi/error.txt", "pi/sleep-midrun.txt"} {
		lines := strings.Split(probeGolden(t, name), "\n")
		if _, bottom := frameCuts(lines); !glyphlessFrame(lines, bottom) {
			t.Errorf("%s: frame not read as glyphless", name)
		}
	}
	for _, name := range []string{"claude/idle-draft.txt", "claude/running-footer.txt", "copilot/idle.txt", "copilot/idle-draft.txt", "copilot/idle-wrapped-80.txt"} {
		lines := strings.Split(probeGolden(t, name), "\n")
		if _, bottom := frameCuts(lines); glyphlessFrame(lines, bottom) {
			t.Errorf("%s: frame read as glyphless", name)
		}
	}
}

// TranscriptCuts: an unambiguous pane has one reading (TranscriptRegion's);
// a draft holding a frame-identical rule adds the other candidate first-rule
// cuts after the chosen one, and each reading's composer is the frame from its
// cut to the bottom rule, the footer left out.
func TestTranscriptCutsListTheChosenCutFirst(t *testing.T) {
	for _, sh := range framedShapes {
		t.Run(sh.golden, func(t *testing.T) {
			golden := probeGolden(t, sh.golden)
			frame := frameRuleOf(t, golden)
			if cuts := TranscriptCuts(golden); len(cuts) != 1 || cuts[0].Region != TranscriptRegion(golden) {
				t.Fatalf("unambiguous pane read as %d cuts", len(cuts))
			}
			pane := withFramedDraft(t, golden, sh.prompt, []string{"draft head", "", frame, "❯ draft tail"})
			cuts := TranscriptCuts(pane)
			if len(cuts) != 2 {
				t.Fatalf("got %d cuts, want 2", len(cuts))
			}
			if cuts[0].Region != TranscriptRegion(golden) {
				t.Fatalf("chosen cut is not the frame's top:\n%s", cuts[0].Region)
			}
			if !strings.Contains(cuts[1].Region, "draft head") || strings.Contains(cuts[1].Region, "draft tail") {
				t.Errorf("alternative cut is not the draft's rule:\n%s", cuts[1].Region)
			}
			if !strings.Contains(cuts[0].Composer, "draft head") || !strings.Contains(cuts[0].Composer, "draft tail") {
				t.Errorf("chosen composer misses the draft:\n%s", cuts[0].Composer)
			}
			if footer := strings.TrimSpace(footerOf(pane)); strings.Contains(cuts[0].Composer, footer) {
				t.Errorf("composer holds the footer %q", footer)
			}
		})
	}
	if cuts := TranscriptCuts(probeGolden(t, "codex/idle.txt")); len(cuts) != 1 || cuts[0].Composer != "" {
		t.Errorf("codex pane read as %d cuts, composer %q", len(cuts), cuts[0].Composer)
	}
}
