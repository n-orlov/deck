package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func probeGolden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "probes", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestTranscriptRegionIgnoresComposerAndFooterChurnForEveryAgentShape (R304):
// each golden below is a real capture of one agent's pane shape. Changing only
// its composer draft or its footer/statusline text leaves the transcript region
// byte-identical, while changing a transcript line changes it. The codex
// goldens have no separator pair at all: their chrome is the "›" composer and
// the status line under it.
func TestTranscriptRegionIgnoresComposerAndFooterChurnForEveryAgentShape(t *testing.T) {
	cases := []struct {
		golden     string
		footerEdit [2]string // replaces footer/statusline text
		draftEdit  [2]string // replaces the composer line, "" when the shape has none to edit
		transcript [2]string // a real transcript change
	}{
		{"codex/idle.txt", [2]string{"fake-model default · /tmp/codex-spike/work", "other-model high · /tmp/elsewhere · renaming... ⠙"},
			[2]string{"› Ask Codex to do anything", "› half a draft"}, [2]string{"hello.txt is the only file", "two files"}},
		{"codex/running.txt", [2]string{"renaming... ⠋", "renaming... ⠙"},
			[2]string{"› Ask Codex to do anything", "› a draft"}, [2]string{"summarise this directory", "summarise that directory"}},
		{"codex/starting.txt", [2]string{"fake-model default", "fake-model high"},
			[2]string{"› Ask Codex to do anything", "› typed"}, [2]string{"OpenAI Codex", "OpenAI Codex!"}},
		{"claude/idle-draft.txt", [2]string{"bypass permissions on (shift+tab to cycle)", "accept edits on (shift+tab to cycle)"},
			[2]string{"❯ now also update the golden for the draft", "❯ a different draft"}, [2]string{"Ran 14 tests, all passed.", "Ran 15 tests, all passed."}},
		{"claude/running-footer.txt", [2]string{"bypass permissions on", "plan mode on"},
			[2]string{"❯ ", "❯ typing"}, [2]string{"I'll refactor the parser next.", "I'll refactor the lexer next."}},
		{"pi/idle.txt", [2]string{"↑7.6k ↓21 $0.039 0.2%/1.0M (auto)", "↑9.9k ↓30 $0.050 0.5%/1.0M (auto)"},
			[2]string{"/tmp/pw1", "/tmp/pw2"}, [2]string{"Ten.", "Eleven."}},
		{"copilot/idle.txt", [2]string{"Manual Approval", "Allow All"},
			[2]string{"❯\n─", "❯ x\n─"}, [2]string{"/tmp/work", "/tmp/elsewhere"}},
		{"copilot/working.txt", [2]string{"◉ Working · 130 B esc interrupt", "◉ Working · 260 B esc interrupt"},
			[2]string{"❯\n─", "❯ x\n─"}, [2]string{"/tmp/work", "/tmp/elsewhere"}},
	}
	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			pane := probeGolden(t, tc.golden)
			region := TranscriptRegion(pane)
			if region == "" {
				t.Fatalf("empty transcript region for %s", tc.golden)
			}
			for name, edit := range map[string][2]string{"footer": tc.footerEdit, "draft": tc.draftEdit} {
				if !strings.Contains(pane, edit[0]) {
					t.Fatalf("%s: golden has no %q", name, edit[0])
				}
				if got := TranscriptRegion(strings.Replace(pane, edit[0], edit[1], 1)); got != region {
					t.Errorf("a %s-only change altered the region:\n%s", name, got)
				}
			}
			if !strings.Contains(pane, tc.transcript[0]) {
				t.Fatalf("golden has no %q", tc.transcript[0])
			}
			if TranscriptRegion(strings.Replace(pane, tc.transcript[0], tc.transcript[1], 1)) == region {
				t.Error("a real transcript change did not alter the region")
			}
		})
	}
}

// Cases of the same rule beyond the goldens' own: a codex approval dialog's
// selected option ("› 1. Yes") is dialog content, not a composer; markdown
// rules in a codex transcript are not a separator pair; a codex pane whose
// composer holds a draft and a status line still excludes both; a claude pane
// ending in a bare prompt (no separators) excludes it.
func TestTranscriptRegionOtherShapes(t *testing.T) {
	waiting := probeGolden(t, "codex/waiting.txt")
	region := TranscriptRegion(waiting)
	if !strings.Contains(region, "› 1. Yes, proceed (y)") || !strings.HasSuffix(region, "Press enter to confirm or esc to cancel") {
		t.Errorf("a codex dialog's option was cut as a composer:\n%s", region)
	}
	if changed := strings.Replace(waiting, "Press enter to confirm", "Press enter to proceed", 1); TranscriptRegion(changed) == region {
		t.Error("a dialog text change did not count")
	}

	rules := "• notes\n---\nmiddle\n---\nmore text\n\n› draft\n\n  model · /tmp\n"
	if got := TranscriptRegion(rules); got != "• notes\n---\nmiddle\n---\nmore text" {
		t.Errorf("codex transcript with markdown rules, region = %q", got)
	}
	if TranscriptRegion(rules) != TranscriptRegion(strings.Replace(rules, "› draft", "› other draft", 1)) {
		t.Error("a codex draft change altered the region")
	}

	bare := "● did it\n\n❯ typing here\n"
	if got := TranscriptRegion(bare); got != "● did it" {
		t.Errorf("claude bare trailing prompt, region = %q", got)
	}
	if TranscriptRegion("● did it\n\n❯ \n") != TranscriptRegion(bare) {
		t.Error("a bare-prompt draft change altered the region")
	}
}
