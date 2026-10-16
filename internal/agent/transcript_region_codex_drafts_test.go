package agent

import (
	"strings"
	"testing"
)

// codexDrafts are composer contents a user can really type into codex: a
// numbered list item (which looks like a dialog option), a multiline draft
// whose continuation lines codex indents, a draft with a blank line inside it,
// and a numbered multiline draft starting at "2.".
var codexDrafts = map[string]string{
	"numbered":            "› 1. Write regression tests",
	"four-line":           "› first line of a draft\n  second line\n  third line\n  fourth line",
	"blank-inside":        "› para one\n\n  para two",
	"numbered-multi-line": "› 2. Cover the footer\n  3. Cover the draft\n  4. Push",
}

// R304: whatever the composer holds, a codex pane's transcript region excludes
// the composer and the status line, so changing only the footer (spinner,
// model, cwd) or only the draft never alters it, and a transcript change does.
func TestCodexTranscriptRegionIgnoresFooterChurnUnderAnyDraft(t *testing.T) {
	shapes := []struct {
		golden, footer, transcript string
	}{
		{"codex/idle.txt", "fake-model default · /tmp/codex-spike/work", "hello.txt is the only file"},
		{"codex/running.txt", "fake-model default · /tmp/codex-spike/work · renaming... ⠋", "summarise this directory"},
		{"codex/retrying.txt", "local-model low · /tmp/codex-spike/work · local-model · Context 0% used", "Reconnecting... 4/5"},
		{"codex/error.txt", "local-model low · /tmp/codex-spike/work · local-model · Context 0% used", "Invalid bearer token"},
	}
	for _, sh := range shapes {
		golden := probeGolden(t, sh.golden)
		for _, s := range []string{sh.footer, sh.transcript, "› Ask Codex to do anything"} {
			if !strings.Contains(golden, s) {
				t.Fatalf("%s has no %q", sh.golden, s)
			}
		}
		base := TranscriptRegion(golden)
		for name, draft := range codexDrafts {
			t.Run(sh.golden+"/"+name, func(t *testing.T) {
				pane := strings.Replace(golden, "› Ask Codex to do anything", draft, 1)
				region := TranscriptRegion(pane)
				if region != base {
					t.Fatalf("the draft leaked into the region:\n%s", region)
				}
				for _, footer := range []string{sh.footer + " · renaming... ⠙", "other-model high · /tmp/elsewhere", sh.footer + " ⠹"} {
					if got := TranscriptRegion(strings.Replace(pane, sh.footer, footer, 1)); got != region {
						t.Errorf("footer %q altered the region:\n%s", footer, got)
					}
				}
				if TranscriptRegion(strings.Replace(pane, sh.transcript, sh.transcript+" (changed)", 1)) == region {
					t.Error("a real transcript change did not alter the region")
				}
			})
		}
	}
}

// The error probe reads above the same composer: a long or numbered draft
// under a terminal error must not hide the error, and one under a working
// pane must not invent one.
func TestCodexTerminalErrorSeesPastAnyDraft(t *testing.T) {
	errPane := probeGolden(t, "codex/error.txt")
	idle := probeGolden(t, "codex/idle.txt")
	for name, draft := range codexDrafts {
		if !codexTerminalError(strings.Replace(errPane, "› Ask Codex to do anything", draft, 1)) {
			t.Errorf("%s draft hid the terminal error", name)
		}
		if codexTerminalError(strings.Replace(idle, "› Ask Codex to do anything", draft, 1)) {
			t.Errorf("%s draft read as a terminal error", name)
		}
	}
}

// A codex selection dialog replaces the composer: its "›" cursor line is
// dialog content wherever the cursor sits, and a cursor move or other dialog
// change still counts as output, for both approval dialogs and the "Press
// enter to continue" shape.
func TestCodexDialogStaysTranscript(t *testing.T) {
	for _, golden := range []string{"codex/waiting.txt", "codex/waiting-patch.txt"} {
		pane := probeGolden(t, golden)
		region := TranscriptRegion(pane)
		if !strings.Contains(region, "› 1. Yes, proceed (y)") {
			t.Fatalf("%s: dialog option cut as a composer:\n%s", golden, region)
		}
		moved := strings.Replace(strings.Replace(pane, "› 1. Yes, proceed (y)", "  1. Yes, proceed (y)", 1), "  2. Yes,", "› 2. Yes,", 1)
		got := TranscriptRegion(moved)
		if got == region || !strings.Contains(got, "› 2. Yes,") {
			t.Errorf("%s: a dialog cursor move was not counted:\n%s", golden, got)
		}
	}
	trust := "• previous turn\n\n  Do you trust the files in this folder?\n\n› 1. Yes, continue\n  2. No, quit\n\n  Press enter to continue\n"
	if got := TranscriptRegion(trust); !strings.HasSuffix(got, "Press enter to continue") || !strings.Contains(got, "› 1. Yes, continue") {
		t.Errorf("trust dialog cut, region = %q", got)
	}
}
