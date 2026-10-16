package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

// framedGolden reads an agent golden and its composer frame rule.
func framedGolden(t *testing.T, name string) (golden, frame string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", name))
	if err != nil {
		t.Fatal(err)
	}
	golden = string(raw)
	for _, line := range strings.Split(golden, "\n") {
		if strings.TrimSpace(line) != "" && strings.Trim(strings.TrimSpace(line), "─") == "" {
			return golden, line
		}
	}
	t.Fatal("golden has no frame")
	return "", ""
}

// assertQuiet fails when a tick changed the row's output/attention state
// against base, or left the row moving.
func (r *movementRig) assertQuiet(now int64, base rowAttention) {
	r.t.Helper()
	got := r.row()
	if got.LastOutputAt != base.output || got.LastSeenAt != base.seen || got.Unseen() != base.unseen ||
		r.clock.Moving(r.id) || got.Status != base.status || got.StatusSource != base.source {
		r.t.Fatalf("draft-only tick at %d changed attention state: output=%d seen=%d unseen=%v moving=%v status=%q source=%q",
			now, got.LastOutputAt, got.LastSeenAt, got.Unseen(), r.clock.Moving(r.id), got.Status, got.StatusSource)
	}
}

type rowAttention struct {
	output, seen   int64
	unseen         bool
	status, source string
}

func (r *movementRig) attention() rowAttention {
	got := r.row()
	return rowAttention{got.LastOutputAt, got.LastSeenAt, got.Unseen(), got.Status, got.StatusSource}
}

// R304/R316/R324: a framed draft quoting a "❯" prompt right under a line drawn
// exactly like the frame (the shape claude's and copilot's own composer top
// has) is still composer content. Adding and editing it across consecutive
// buckets leaves last_output_at, last_seen_at, unseen, movement and status
// alone; real transcript changes above it still count and enable movement, and
// editing the draft after that output counts nothing either.
func TestQuotedPromptDraftLeavesOutputClockAndMovementAlone(t *testing.T) {
	shapes := []struct{ kind, golden, prompt, transcript string }{
		{"pi", "pi/idle.txt", "", "Ten."},
		{"claude", "claude/idle-draft.txt", "❯ ", "Ran 14 tests, all passed."},
		{"copilot", "copilot/idle-draft.txt", "❯ ", "MCP Servers reloaded"},
	}
	for _, sh := range shapes {
		golden, frame := framedGolden(t, sh.golden)
		draft := func(head, quoted, tail string) string {
			return spliceFramedDraft(t, golden, []string{sh.prompt + head, "", frame, "❯ " + quoted, tail})
		}
		t.Run(sh.kind, func(t *testing.T) {
			r := newMovementRig(t, sh.kind, 2*time.Hour)
			r.tickPane(1000, 1000, golden)
			base := r.attention()
			edits := [][3]string{
				{"ordinary draft text", "quoted prompt, not an agent turn", "trailing draft text"},
				{"ordinary draft text, edited", "quoted prompt, not an agent turn", "trailing draft text"},
				{"ordinary draft text, edited", "quoted prompt, edited", "trailing draft text, edited"},
			}
			for i, e := range edits {
				now := int64(1010 + 10*i)
				r.tickPane(now, now, draft(e[0], e[1], e[2]))
				r.assertQuiet(now, base)
			}
			pane := draft(edits[2][0], edits[2][1], edits[2][2])
			r.tickPane(1040, 1040, strings.Replace(pane, sh.transcript, sh.transcript+" two", 1))
			if got := r.row(); got.LastOutputAt != 1040000 || !got.Unseen() {
				t.Fatalf("a transcript change did not count: last_output_at=%d unseen=%v", got.LastOutputAt, got.Unseen())
			}
			after := strings.Replace(pane, sh.transcript, sh.transcript+" three", 1)
			r.tickPane(1050, 1050, after)
			if !r.clock.Moving(r.id) {
				t.Fatal("two real transcript changes did not enable movement")
			}
			r.wantStatus("running", "transcript changes in two consecutive buckets")
			for i, head := range []string{"head after output", "head again"} {
				now := int64(1060 + 10*i)
				edited := strings.Replace(after, sh.prompt+edits[2][0], sh.prompt+head, 1)
				if edited == after {
					t.Fatal("edit did not apply")
				}
				r.tickPane(now, now, edited)
				if got := r.row(); got.LastOutputAt != 1050000 {
					t.Fatalf("draft edit after output counted at %d: last_output_at=%d", now, got.LastOutputAt)
				}
			}
		})
	}
}

// R304/R316/R324: the clock does not rely on the boundary rule alone. Here pi's
// transcript runs right up to the frame (no blank line above it) and the draft
// holds a blank line and then a frame-identical rule, so the pane read cold
// cannot tell the frame's top rule from the draft's (TranscriptRegion picks the
// draft's). Once the clock has read the frame, a draft-only edit is still not
// output, whichever rule it mimics, and output above the draft keeps counting.
func TestOutputClockTracksTheFrameThroughAMimickingDraft(t *testing.T) {
	golden, frame := framedGolden(t, "pi/idle.txt")
	tight := strings.Replace(golden, " Ten.\n\n"+frame, " Ten.\n"+frame, 1)
	if tight == golden {
		t.Fatal("golden lost its blank line above the frame")
	}
	draft := func(transcript, head, tail string) string {
		pane := spliceFramedDraft(t, tight, []string{head, "", frame, tail})
		return strings.Replace(pane, " Ten.", " "+transcript, 1)
	}
	if agent.TranscriptRegion(draft("Ten.", "a", "b")) == agent.TranscriptRegion(tight) {
		t.Fatal("the pane no longer misleads the cold boundary rule; the test proves nothing about the clock")
	}
	r := newMovementRig(t, "pi", 2*time.Hour)
	r.tickPane(1000, 1000, tight)
	base := r.attention()
	for i, d := range [][2]string{{"a", "b"}, {"a edited", "b"}, {"a edited", "b edited"}} {
		now := int64(1010 + 10*i)
		r.tickPane(now, now, draft("Ten.", d[0], d[1]))
		r.assertQuiet(now, base)
	}
	r.tickPane(1040, 1040, draft("Ten. Eleven.", "a edited", "b edited"))
	if got := r.row(); got.LastOutputAt != 1040000 {
		t.Fatalf("output above the draft did not count: last_output_at=%d", got.LastOutputAt)
	}
	r.tickPane(1050, 1050, draft("Ten. Eleven. Twelve.", "a edited", "b edited"))
	if !r.clock.Moving(r.id) {
		t.Fatal("two real transcript changes did not enable movement")
	}
	for i, head := range []string{"a after output", "a once more"} {
		now := int64(1060 + 10*i)
		r.tickPane(now, now, draft("Ten. Eleven. Twelve.", head, "b edited"))
		if got := r.row(); got.LastOutputAt != 1050000 {
			t.Fatalf("draft edit after output counted at %d: last_output_at=%d", now, got.LastOutputAt)
		}
	}
}
