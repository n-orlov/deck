package agent

import (
	"strings"
	"testing"
)

// R304: pi pads its transcript by one column, but a draft is free text, so
// its own indented lines are no evidence that the rule above them is
// transcript. Each draft below holds indentation next to a line drawn exactly
// like the frame ("@"); "%s" is the head the test edits. The first is the
// shape review found; the others are the same rule in shapes it did not list.
var piIndentedDraftShapes = map[string][]string{
	"finding":          {"%s", "  preserve this indentation", "", "@", "appendix"},
	"indented-head":    {"  %s", "", "@", "tail"},
	"pasted-code":      {"%s", "    for x in items:", "\tprint(x)", "@", "  more code", "", "@", "end"},
	"indent-under-at":  {"%s", "@", "   indented under a drawn rule", "end"},
	"one-col-and-col0": {"%s", " one column, but under a column-0 line", "@", "end"},
}

func fillPiDraft(shape []string, frame, head string) []string {
	out := make([]string, 0, len(shape))
	for _, line := range shape {
		switch {
		case line == "@":
			line = frame
		case strings.Contains(line, "%s"):
			line = strings.Replace(line, "%s", head, 1)
		}
		out = append(out, line)
	}
	return out
}

func TestPiIndentedDraftIsNeverTranscript(t *testing.T) {
	for name, line := range map[string]string{"pi/idle.txt": " Ten.", "pi/running.txt": " run: echo hello-from-tool"} {
		golden := probeGolden(t, name)
		frame := frameRuleOf(t, golden)
		bases := map[string]string{
			"plain":        golden,
			"divider":      withTranscriptTail(t, golden, []string{"@", " after divider"}),
			"two-dividers": withTranscriptTail(t, golden, []string{"@", " first", "", "@", " after divider"}),
		}
		for baseName, base := range bases {
			want := TranscriptRegion(base)
			for shapeName, shape := range piIndentedDraftShapes {
				t.Run(name+"/"+baseName+"/"+shapeName, func(t *testing.T) {
					for _, head := range []string{"please explain", "please explain again"} {
						pane := withFramedDraft(t, base, "", fillPiDraft(shape, frame, head))
						if got := TranscriptRegion(pane); got != want {
							t.Fatalf("draft with head %q moved the cut:\n%s", head, got)
						}
					}
					pane := withFramedDraft(t, base, "", fillPiDraft(shape, frame, "please explain"))
					changed := strings.Replace(pane, line+"\n", line+" More.\n", 1)
					if changed == pane {
						t.Fatal("golden lacks the transcript line the test changes")
					}
					for _, cut := range TranscriptCuts(changed) {
						if cut.Region == want {
							t.Fatal("a transcript change above the draft is hidden by one of the cuts")
						}
					}
				})
			}
		}
	}
}
