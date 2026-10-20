package agent

import (
	"fmt"
	"strings"
	"testing"
)

// R304: an agent's working indicator (spinner frame, elapsed/retry counters) is
// statusline churn above the composer, not output. Each golden below is a real
// capture; turning only the indicator over leaves the transcript region alone,
// while a transcript line, or the indicator appearing, still changes it.
func TestTranscriptRegionIgnoresWorkingIndicatorChurn(t *testing.T) {
	cases := []struct {
		golden, from, to string
	}{
		{"pi/running.txt", "⠧ Working...", "⠏ Working..."},
		{"pi/sleep-midrun.txt", "⠴ Working...", "⠦ Working..."},
		{"pi/sleep-midrun.txt", "Elapsed 11.0s", "Elapsed 12.0s"},
		{"claude/running-footer.txt", "✳ Working… (12s ·", "✶ Working… (13s ·"},
		{"claude/running.txt", "✳ Working…", "✻ Working…"},
		{"codex/running.txt", "Working (5s •", "Working (6s •"},
		{"codex/retrying.txt", "Reconnecting... 4/5 (2s •", "Reconnecting... 4/5 (3s •"},
	}
	for _, tc := range cases {
		t.Run(tc.golden+"/"+tc.to, func(t *testing.T) {
			pane := probeGolden(t, tc.golden)
			if !strings.Contains(pane, tc.from) {
				t.Fatalf("golden has no %q", tc.from)
			}
			region := TranscriptRegion(pane)
			if got := TranscriptRegion(strings.Replace(pane, tc.from, tc.to, 1)); got != region {
				t.Errorf("indicator churn altered the region:\n%s\n--- was\n%s", got, region)
			}
		})
	}

	pane := probeGolden(t, "pi/running.txt")
	region := TranscriptRegion(pane)
	if TranscriptRegion(strings.Replace(pane, "run: echo hello-from-tool", "run: echo other", 1)) == region {
		t.Error("a tool line change did not alter the region")
	}
	idle := strings.Replace(pane, " ⠧ Working...\n", "", 1)
	if TranscriptRegion(idle) == region {
		t.Error("the indicator vanishing did not alter the region")
	}
	if TranscriptRegion(strings.Replace(pane, "⠧ Working...", "⠧ Working... and done", 1)) == region {
		t.Error("new text on the indicator line did not alter the region")
	}
}

// R304: prose that only reads like an indicator is transcript; its numbers
// changing changes the region. Only the exact indicator shapes are masked.
func TestTranscriptRegionCountsIndicatorLikeProse(t *testing.T) {
	cases := []struct {
		golden, from, line string
	}{
		{"codex/idle.txt", "• I listed the directory: hello.txt is the only file.", "• Working on ticket %d."},
		{"codex/idle.txt", "• I listed the directory: hello.txt is the only file.", "• Running migration step %d of 9"},
		{"pi/idle.txt", " Ten.", " Elapsed %d seconds during the completed benchmark."},
		{"pi/idle.txt", " Ten.", " Elapsed %d.0s"},
		{"claude/idle-draft.txt", "  Ran 14 tests, all passed.", "  Thinking about test %d next."},
		{"pi/running.txt", "run: echo hello-from-tool", "Working... step %d"},
		{"codex/running.txt", "summarise this directory", "Reconnecting... %d/5"},
		{"pi/running.txt", " run: echo hello-from-tool", " Benchmark finished:\n\n ```text\n Elapsed %d.0s\n ```\n\n run: echo hello-from-tool"},
		{"pi/sleep-midrun.txt", " run: sleep 25", " $ make bench\n\n Elapsed %d.5s\n\n run: sleep 25"},
		{"claude/running-footer.txt", "  I'll refactor the parser next.", "✻ Worked… (%ds · esc to interrupt)\n  I'll refactor the parser next."},
		{"codex/running.txt", "⚠ Model metadata", "• Working (%ds • esc to interrupt)\n⚠ Model metadata"},
		{"pi/running.txt", " run: echo hello-from-tool", " $ make bench\n\n Elapsed %d.5s\n\n run: echo hello-from-tool"},
		{"claude/idle-draft.txt", "  Done. The status parser", "✻ Worked… (%ds · esc to interrupt)\n⏺ Done. The status parser"},
		{"codex/idle.txt", "⚠ Model metadata", "• Working (%ds • esc to interrupt)\n⚠ Model metadata"},
		{"codex/retrying.txt", "• Reconnecting... 4/5", "• Reconnecting... %d/5 (9s • esc to interrupt)\n• Reconnecting... 4/5"},
	}
	for _, tc := range cases {
		t.Run(tc.golden+"/"+tc.line, func(t *testing.T) {
			pane := probeGolden(t, tc.golden)
			if !strings.Contains(pane, tc.from) {
				t.Fatalf("golden has no %q", tc.from)
			}
			a := TranscriptRegion(strings.Replace(pane, tc.from, fmt.Sprintf(tc.line, 41), 1))
			b := TranscriptRegion(strings.Replace(pane, tc.from, fmt.Sprintf(tc.line, 42), 1))
			if a == b {
				t.Errorf("a transcript number change left the region unchanged:\n%s", a)
			}
		})
	}
}
