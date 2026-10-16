package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// spliceFramedDraft replaces the composer between the pane's last two
// separator rules with draft, as a user typing into claude, pi or copilot
// would see it.
func spliceFramedDraft(t *testing.T, pane string, draft []string) string {
	t.Helper()
	lines := strings.Split(pane, "\n")
	var rules []int
	for i := len(lines) - 1; i >= 0 && len(rules) < 2; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" && strings.Trim(trimmed, "─") == "" {
			rules = append(rules, i)
		}
	}
	if len(rules) < 2 {
		t.Fatal("golden has no composer frame")
	}
	out := append(append(append([]string{}, lines[:rules[1]+1]...), draft...), lines[rules[0]:]...)
	return strings.Join(out, "\n")
}

// lastNonBlankLine is the pane's footer line under the composer frame.
func lastNonBlankLine(pane string) string {
	lines := strings.Split(pane, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

// R304/R316/R324 for the framed composer (claude, pi, copilot): a draft that
// holds a rule line ("---", a box rule) is still composer content. Editing the
// draft's first line and churning the footer in consecutive buckets leaves
// last_output_at, the unseen marker and the row's movement status alone; a real
// transcript change in two consecutive buckets still counts.
func TestFramedDraftWithRuleLineLeavesOutputClockUnseenAndMovementAlone(t *testing.T) {
	shapes := []struct {
		kind, golden, prompt, transcript string
	}{
		{"claude", "claude/idle-draft.txt", "❯ ", "Ran 14 tests, all passed."},
		{"pi", "pi/idle.txt", "", "Ten."},
		{"copilot", "copilot/idle-draft.txt", "❯ ", "MCP Servers reloaded"},
	}
	drafts := map[string][]string{
		"markdown-rule": {"write a markdown summary", "  ---", "  keep this draft unsent"},
		"trailing-rule": {"the draft ends in a rule", "  ---"},
		"column-0-rule": {"notes", "-----", "more notes"},
		"box-rule":      {"a box rule", "  ──────────"},
	}
	for _, sh := range shapes {
		raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", sh.golden))
		if err != nil {
			t.Fatal(err)
		}
		for name, draft := range drafts {
			t.Run(sh.kind+"/"+name, func(t *testing.T) {
				typed := func(first string) string {
					body := append([]string{sh.prompt + first}, draft[1:]...)
					return spliceFramedDraft(t, string(raw), body)
				}
				idle := typed(draft[0])
				if !strings.Contains(idle, sh.transcript) || !strings.Contains(idle, draft[1]) {
					t.Fatal("golden lost its transcript or the draft")
				}
				footer := lastNonBlankLine(idle)
				r := newMovementRig(t, sh.kind, 2*time.Hour)
				r.tickPane(1000, 1000, idle)
				base := r.row()
				for i, edit := range []string{"rewrite " + draft[0], "rewrite it all: " + draft[0], "x"} {
					now := int64(1010 + 10*i)
					pane := typed(edit)
					if i == 2 {
						pane = strings.Replace(pane, footer, footer+" ctx:42%", 1)
					}
					r.tickPane(now, now, pane)
					got := r.row()
					if got.LastOutputAt != 0 || got.Unseen() || got.Status != base.Status || got.StatusSource == "movement" {
						t.Fatalf("composer/footer-only tick at %d: last_output_at=%d unseen=%v status=%q source=%q (base %q)",
							now, got.LastOutputAt, got.Unseen(), got.Status, got.StatusSource, base.Status)
					}
				}
				r.tickPane(1040, 1040, strings.Replace(idle, sh.transcript, sh.transcript+" two", 1))
				if got := r.row(); got.LastOutputAt == 0 || !got.Unseen() {
					t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
				}
				r.tickPane(1050, 1050, strings.Replace(idle, sh.transcript, sh.transcript+" three", 1))
				r.wantStatus("running", "transcript changes in two consecutive buckets")
			})
		}
	}
}
