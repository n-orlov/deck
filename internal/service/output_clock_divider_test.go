package service

import (
	"strings"
	"testing"
	"time"
)

// dividerPane appends a transcript block to golden's transcript, right above
// its composer frame: a line drawn exactly like the frame, then what the agent
// draws under a transcript line (text names the line the tests change).
func dividerPane(t *testing.T, golden, frame string, tail []string) string {
	t.Helper()
	lines := strings.Split(golden, "\n")
	top, n := -1, 0
	for i := len(lines) - 1; i >= 0 && top < 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" && strings.Trim(trimmed, "─") == "" {
			if n++; n == 2 {
				top = i
			}
		}
	}
	if top < 0 {
		t.Fatal("golden has no composer frame")
	}
	body := []string{}
	for _, line := range tail {
		if line == "@" {
			line = frame
		}
		body = append(body, line)
	}
	body = append(body, "")
	return strings.Join(append(append(append([]string{}, lines[:top]...), body...), lines[top:]...), "\n")
}

// R304/R316/R324: a transcript holding a line drawn exactly like the composer
// frame is transcript, whether the clock meets it on its first capture of the
// session (cold) or sees it arrive after a baseline (warm). Output under the
// divider counts and, in two consecutive buckets, enables movement; draft
// edits, with or without a frame-mimicking rule, and footer churn never do.
func TestFrameIdenticalTranscriptDividerCountsAsOutput(t *testing.T) {
	shapes := []struct {
		kind, golden, prompt string
		tail                 []string
	}{
		{"pi", "pi/idle.txt", "", []string{"@", " under the divider"}},
		{"pi", "pi/running.txt", "", []string{"@", "", " under the divider"}},
		{"claude", "claude/idle-draft.txt", "❯ ", []string{"@", "● under the divider"}},
		{"claude", "claude/running-footer.txt", "❯ ", []string{"@", "❯ an earlier question", "", "● under the divider"}},
		{"copilot", "copilot/idle-draft.txt", "❯ ", []string{"@", " ● under the divider"}},
	}
	for _, sh := range shapes {
		golden, frame := framedGolden(t, sh.golden)
		withDivider := dividerPane(t, golden, frame, sh.tail)
		typed := func(pane, head string) string {
			return spliceFramedDraft(t, pane, []string{sh.prompt + head, "", frame, "keep this unsent"})
		}
		for _, mode := range []string{"cold", "warm"} {
			t.Run(sh.golden+"/"+mode, func(t *testing.T) {
				r := newMovementRig(t, sh.kind, 2*time.Hour)
				start := withDivider
				if mode == "warm" {
					start = golden
				}
				r.tickPane(1000, 1000, start)
				base := r.attention()
				if mode == "warm" {
					// The divider and its text are new output.
					r.tickPane(1010, 1010, withDivider)
					if got := r.row(); got.LastOutputAt != 1010000 || !got.Unseen() {
						t.Fatalf("a divider and the text under it did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
					}
					base = r.attention()
				} else {
					for i, head := range []string{"write an outline", "rewrite the outline"} {
						now := int64(1010 + 10*i)
						r.tickPane(now, now, typed(withDivider, head))
						r.assertQuiet(now, base)
					}
				}
				changed := strings.Replace(withDivider, "under the divider", "under the divider, one", 1)
				r.tickPane(1040, 1040, typed(changed, "rewrite it all"))
				if got := r.row(); got.LastOutputAt != 1040000 || !got.Unseen() || got.LastSeenAt != base.seen {
					t.Fatalf("output under the divider did not count: last_output_at=%d unseen=%v seen=%d", got.LastOutputAt, got.Unseen(), got.LastSeenAt)
				}
				again := strings.Replace(withDivider, "under the divider", "under the divider, two", 1)
				r.tickPane(1050, 1050, typed(again, "rewrite it all"))
				if !r.clock.Moving(r.id) {
					t.Fatal("two real transcript changes did not enable movement")
				}
				r.wantStatus("running", "transcript changes in two consecutive buckets")
				for i, head := range []string{"head after output", "head again"} {
					now := int64(1060 + 10*i)
					r.tickPane(now, now, typed(again, head))
					if got := r.row(); got.LastOutputAt != 1050000 {
						t.Fatalf("draft edit after output counted at %d: last_output_at=%d", now, got.LastOutputAt)
					}
				}
			})
		}
	}
}
