package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R304/R316/R324 for every agent pane shape the repo holds a golden of: after a
// first sight, footer/statusline-only edits in consecutive buckets leave
// last_output_at, last_seen_at, the unseen marker, movement and the status
// alone; a real transcript edit afterwards still counts, in two consecutive
// buckets enables movement.
func TestFooterOnlyChurnInEveryAgentGoldenLeavesOutputClockAlone(t *testing.T) {
	cases := []struct {
		kind, golden string
		footer       [2]string // footer/statusline text and its churned variants
		variants     []string
		transcript   string
	}{
		{"codex", "codex/idle.txt", [2]string{"fake-model default · /tmp/codex-spike/work", ""},
			[]string{"fake-model default · /tmp/codex-spike/work · renaming... ⠋", "fake-model high · /tmp/elsewhere · Context 4% used"}, "hello.txt is the only file"},
		{"codex", "codex/starting.txt", [2]string{"fake-model default", ""},
			[]string{"fake-model high", "fake-model high · Context 9% used"}, "OpenAI Codex"},
		{"codex", "codex/error.txt", [2]string{"local-model low · /tmp/codex-spike/work · local-model · Context 0% used", ""},
			[]string{"local-model low · /tmp/codex-spike/work · local-model · Context 3% used", "other · /tmp/x · Context 9% used"}, "hello"},
		{"claude", "claude/idle-draft.txt", [2]string{"bypass permissions on (shift+tab to cycle)", ""},
			[]string{"accept edits on (shift+tab to cycle)", "plan mode on (shift+tab to cycle)"}, "Ran 14 tests, all passed."},
		{"claude", "claude/idle-altscreen.txt", [2]string{"bypass permissions on (shift+tab to cycle)", ""},
			[]string{"accept edits on (shift+tab to cycle)", "plan mode on (shift+tab to cycle)"}, ""},
		{"pi", "pi/idle.txt", [2]string{"↑7.6k ↓21 $0.039 0.2%/1.0M (auto)", ""},
			[]string{"↑9.9k ↓30 $0.050 0.5%/1.0M (auto)", "↑11k ↓44 $0.060 0.9%/1.0M (auto)"}, "Ten."},
		{"copilot", "copilot/idle.txt", [2]string{"Manual Approval", ""},
			[]string{"Allow All", "Autopilot"}, "/tmp/work"},
	}
	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			pane := string(raw)
			if !strings.Contains(pane, tc.footer[0]) {
				t.Fatalf("golden has no footer %q", tc.footer[0])
			}
			r := newMovementRig(t, tc.kind, 2*time.Hour)
			r.tickPane(1000, 1000, pane)
			base := r.attention()
			for i, v := range tc.variants {
				now := int64(1010 + 10*i)
				r.tickPane(now, now, strings.Replace(pane, tc.footer[0], v, 1))
				r.assertQuiet(now, base)
			}
			if tc.transcript == "" {
				return
			}
			if !strings.Contains(pane, tc.transcript) {
				t.Fatalf("golden has no transcript text %q", tc.transcript)
			}
			r.tickPane(1040, 1040, strings.Replace(pane, tc.transcript, tc.transcript+" (changed)", 1))
			if got := r.row(); got.LastOutputAt == 0 || !got.Unseen() {
				t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
			}
		})
	}
}

// R304/R316/R324: a working indicator above the composer (spinner frame,
// elapsed or retry counter) is statusline churn. Turning it over in
// consecutive buckets leaves last_output_at, the unseen marker and the status
// alone; a changed tool line still counts.
func TestWorkingIndicatorChurnLeavesOutputClockAlone(t *testing.T) {
	cases := []struct {
		kind, golden string
		churn        [][2]string
		real         [2]string
	}{
		{"pi", "pi/running.txt", [][2]string{{"⠧ Working...", "⠏ Working..."}, {"⠧ Working...", "⠴ Working..."}},
			[2]string{"run: echo hello-from-tool", "run: echo other"}},
		{"pi", "pi/sleep-midrun.txt", [][2]string{{"Elapsed 11.0s", "Elapsed 12.0s"}, {"Elapsed 11.0s", "Elapsed 13.5s"}},
			[2]string{"run: sleep 25", "run: sleep 26"}},
		{"claude", "claude/running-footer.txt", [][2]string{{"✳ Working… (12s ·", "✶ Working… (13s ·"}, {"✳ Working… (12s ·", "✻ Working… (14s ·"}},
			[2]string{"I'll refactor the parser next.", "I'll refactor the lexer next."}},
		{"codex", "codex/running.txt", [][2]string{{"Working (5s •", "Working (6s •"}, {"Working (5s •", "Working (7s •"}},
			[2]string{"summarise this directory", "summarise that directory"}},
		{"codex", "codex/retrying.txt", [][2]string{{"4/5 (2s •", "4/5 (3s •"}, {"4/5 (2s •", "4/5 (4s •"}},
			[2]string{"Invalid bearer token", "Expired bearer token"}},
	}
	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			pane := string(raw)
			r := newMovementRig(t, tc.kind, 2*time.Hour)
			r.tickPane(1000, 1000, pane)
			base := r.attention()
			for i, c := range tc.churn {
				if !strings.Contains(pane, c[0]) {
					t.Fatalf("golden has no %q", c[0])
				}
				now := int64(1010 + 10*i)
				r.tickPane(now, now, strings.Replace(pane, c[0], c[1], 1))
				r.assertQuiet(now, base)
			}
			if !strings.Contains(pane, tc.real[0]) {
				t.Fatalf("golden has no %q", tc.real[0])
			}
			r.tickPane(1040, 1040, strings.Replace(pane, tc.real[0], tc.real[1], 1))
			if got := r.row(); got.LastOutputAt == 0 || !got.Unseen() {
				t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
			}
		})
	}
}
