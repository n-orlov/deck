package agent

import (
	"strings"
	"testing"
)

// R304 (review F01): the working indicator's leading glyph is animation. Codex
// blinks its bullet between • and ◦, claude cycles · ✢ ✳ ✶ ✻ ✽, so whichever
// glyph the frame draws, turning only the glyph over never alters the region;
// the indicator's phrase is still transcript, and a bullet change on a line
// that is not the live indicator still counts.
func TestTranscriptRegionIgnoresWorkingIndicatorGlyphChurn(t *testing.T) {
	cases := []struct {
		golden, from string
		glyphs       []string
	}{
		{"codex/running.txt", "• Working (5s", []string{"◦", "○", "●", "·", "*", "✳"}},
		{"codex/retrying.txt", "• Reconnecting... 4/5 (2s", []string{"◦", "○", "·"}},
		{"claude/running-footer.txt", "✳ Working… (12s", []string{"·", "✢", "✶", "✻", "✽", "◦", "•"}},
		{"claude/running.txt", "✳ Working…", []string{"·", "✢", "✶", "✻", "✽", "◦"}},
	}
	for _, tc := range cases {
		pane := probeGolden(t, tc.golden)
		if !strings.Contains(pane, tc.from) {
			t.Fatalf("%s has no %q", tc.golden, tc.from)
		}
		region := TranscriptRegion(pane)
		_, rest, _ := strings.Cut(tc.from, " ")
		rest = " " + rest
		for _, g := range tc.glyphs {
			t.Run(tc.golden+"/"+g, func(t *testing.T) {
				if got := TranscriptRegion(strings.Replace(pane, tc.from, g+rest, 1)); got != region {
					t.Errorf("glyph %q altered the region:\n%s", g, got)
				}
			})
		}
		phrase := strings.Replace(pane, tc.from, "◦"+strings.Replace(rest, "Working", "Waiting", 1), 1)
		if strings.Contains(tc.from, "Working") && TranscriptRegion(phrase) == region {
			t.Errorf("%s: a changed indicator phrase did not alter the region", tc.golden)
		}
	}

	idle := probeGolden(t, "codex/idle.txt")
	region := TranscriptRegion(idle)
	line := "• I listed the directory: hello.txt is the only file."
	if !strings.Contains(idle, line) {
		t.Fatalf("codex/idle.txt has no %q", line)
	}
	if TranscriptRegion(strings.Replace(idle, line, "◦"+line[len("•"):], 1)) == region {
		t.Error("a bullet change on a completed agent line did not alter the region")
	}
}
