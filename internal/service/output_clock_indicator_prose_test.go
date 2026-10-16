package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R304/R316/R324: only the exact working-indicator shapes the agents draw are
// statusline churn. Transcript prose that merely reads like one ("• Working on
// ticket 41.", "Elapsed 41 seconds …", a codex "• Running …" cell) is output:
// changing its numbers sets last_output_at and the unseen marker, and in two
// consecutive buckets makes the row movement-eligible (running).
func TestIndicatorLikeTranscriptNumbersStillCount(t *testing.T) {
	cases := []struct {
		name, kind, golden, from, line string // line is a format with one %d
	}{
		{"codex-assistant-working-prose", "codex", "codex/idle.txt",
			"• I listed the directory: hello.txt is the only file.", "• Working on ticket %d."},
		{"pi-elapsed-prose", "pi", "pi/idle.txt", " Ten.", " Elapsed %d seconds during the completed benchmark."},
		{"pi-elapsed-counter-shape-while-idle", "pi", "pi/idle.txt", " Ten.", " Elapsed %d.0s"},
		{"codex-running-cell", "codex", "codex/idle.txt",
			"• I listed the directory: hello.txt is the only file.", "• Running migration step %d of 9"},
		{"claude-thinking-prose", "claude", "claude/idle-draft.txt", "  Ran 14 tests, all passed.", "  Thinking about test %d next."},
		{"pi-working-prose-above-live-spinner", "pi", "pi/running.txt", "run: echo hello-from-tool", "Working... step %d"},
		{"codex-reconnecting-prose-above-live-indicator", "codex", "codex/running.txt", "summarise this directory", "Reconnecting... %d/5"},
		// A completed counter- or indicator-shaped line further up the
		// transcript is output even while a live indicator is drawn below it:
		// only the indicator at the bottom of the region is churn.
		{"pi-completed-elapsed-above-live-spinner", "pi", "pi/running.txt", " run: echo hello-from-tool",
			" Benchmark finished:\n\n ```text\n Elapsed %d.0s\n ```\n\n run: echo hello-from-tool"},
		{"pi-earlier-tool-elapsed-above-running-tool-counter", "pi", "pi/sleep-midrun.txt", " run: sleep 25",
			" $ make bench\n\n Elapsed %d.5s\n\n run: sleep 25"},
		{"claude-completed-indicator-line-above-live-indicator", "claude", "claude/running-footer.txt", "  I'll refactor the parser next.",
			"✻ Worked… (%ds · esc to interrupt)\n  I'll refactor the parser next."},
		{"codex-earlier-working-cell-above-live-indicator", "codex", "codex/running.txt", "⚠ Model metadata",
			"• Working (%ds • esc to interrupt)\n⚠ Model metadata"},
		{"pi-completed-tool-elapsed-above-live-spinner", "pi", "pi/running.txt", " run: echo hello-from-tool",
			" $ make bench\n\n Elapsed %d.5s\n\n run: echo hello-from-tool"},
		{"claude-completed-indicator-line-while-idle", "claude", "claude/idle-draft.txt", "  Done. The status parser",
			"✻ Worked… (%ds · esc to interrupt)\n⏺ Done. The status parser"},
		{"codex-earlier-working-cell-while-idle", "codex", "codex/idle.txt", "⚠ Model metadata",
			"• Working (%ds • esc to interrupt)\n⚠ Model metadata"},
		{"codex-earlier-reconnecting-cell-above-live-indicator", "codex", "codex/retrying.txt", "• Reconnecting... 4/5",
			"• Reconnecting... %d/5 (9s • esc to interrupt)\n• Reconnecting... 4/5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			pane := string(raw)
			if !strings.Contains(pane, tc.from) {
				t.Fatalf("golden has no %q", tc.from)
			}
			at := func(n int) string { return strings.Replace(pane, tc.from, fmt.Sprintf(tc.line, n), 1) }
			r := newMovementRig(t, tc.kind, 2*time.Hour)
			r.tickPane(1000, 1000, at(41))
			if got := r.row(); got.LastOutputAt != 0 || got.Unseen() {
				t.Fatalf("first sight counted as output: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
			}
			r.tickPane(1010, 1010, at(42))
			if got := r.row(); got.LastOutputAt != 1010_000 || !got.Unseen() {
				t.Fatalf("a transcript number change at 1010 did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
			}
			r.tickPane(1020, 1020, at(43))
			// Two consecutive counted buckets: movement-eligible, so the row is
			// running (and a running row is never unseen, store.Session.Unseen).
			if got := r.row(); got.LastOutputAt != 1020_000 || !r.clock.Moving(r.id) || got.Status != "running" {
				t.Fatalf("transcript changes in two consecutive buckets: last_output_at=%d moving=%v status=%q, want 1020000/true/running",
					got.LastOutputAt, r.clock.Moving(r.id), got.Status)
			}
		})
	}
}
