package agent

import (
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
