package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R304/R316/R324: a framed draft holding a line drawn exactly like the frame's
// rules is composer content in every framed shape. Adding and editing such
// drafts across consecutive buckets leaves last_output_at, last_seen_at, the
// unseen marker, movement and status alone; real transcript changes in two
// consecutive buckets still advance the clock and enable movement.
func TestFrameIdenticalDraftLeavesOutputClockAndMovementAlone(t *testing.T) {
	shapes := []struct {
		kind, golden, prompt, transcript string
	}{
		{"pi", "pi/idle.txt", "", "Ten."},
		{"claude", "claude/idle-draft.txt", "❯ ", "Ran 14 tests, all passed."},
		{"copilot", "copilot/idle-draft.txt", "❯ ", "MCP Servers reloaded"},
	}
	for _, sh := range shapes {
		raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", sh.golden))
		if err != nil {
			t.Fatal(err)
		}
		golden := string(raw)
		var frame string
		for _, line := range strings.Split(golden, "\n") {
			if strings.TrimSpace(line) != "" && strings.Trim(strings.TrimSpace(line), "─") == "" {
				frame = line
				break
			}
		}
		if frame == "" {
			t.Fatal("golden has no frame")
		}
		drafts := map[string][]string{
			"rule-between-text": {"", frame, "keep this unsent"},
			"rule-after-blank":  {"", "", frame, "keep this unsent"},
			"rule-twice":        {"", frame, "mid", frame, "tail"},
		}
		for name, tail := range drafts {
			t.Run(sh.kind+"/"+name, func(t *testing.T) {
				typed := func(first string) string {
					body := append([]string{sh.prompt + first}, tail[1:]...)
					return spliceFramedDraft(t, golden, body)
				}
				r := newMovementRig(t, sh.kind, 2*time.Hour)
				r.tickPane(1000, 1000, golden)
				base := r.row()
				for i, first := range []string{"write an outline", "rewrite the outline", "rewrite it all"} {
					now := int64(1010 + 10*i)
					r.tickPane(now, now, typed(first))
					got := r.row()
					if got.LastOutputAt != base.LastOutputAt || got.LastSeenAt != base.LastSeenAt || got.Unseen() != base.Unseen() ||
						r.clock.Moving(r.id) || got.Status != base.Status || got.StatusSource != base.StatusSource || got.StatusAt != base.StatusAt {
						t.Fatalf("draft-only tick at %d changed attention state: output=%d seen=%d unseen=%v status=%q source=%q",
							now, got.LastOutputAt, got.LastSeenAt, got.Unseen(), got.Status, got.StatusSource)
					}
				}
				idle := typed("rewrite it all")
				r.tickPane(1040, 1040, strings.Replace(idle, sh.transcript, sh.transcript+" two", 1))
				if got := r.row(); got.LastOutputAt == 0 || !got.Unseen() {
					t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
				}
				r.tickPane(1050, 1050, strings.Replace(idle, sh.transcript, sh.transcript+" three", 1))
				if !r.clock.Moving(r.id) {
					t.Fatal("two real transcript changes did not enable movement")
				}
				r.wantStatus("running", "transcript changes in two consecutive buckets")
			})
		}
	}
}
