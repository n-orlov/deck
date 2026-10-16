package service

import (
	"strings"
	"testing"
	"time"
)

// piIndentedDrafts are pi drafts that hold indented lines next to a line drawn
// exactly like the frame. "%s" is the head the tests edit; "@" is the frame
// rule. The first is the shape review found; the others are the same rule
// (indentation in a draft is not transcript evidence) in shapes it did not list.
var piIndentedDrafts = map[string][]string{
	"finding":         {"%s", "  preserve this indentation", "", "@", "appendix"},
	"indented-head":   {"  %s", "", "@", "tail"},
	"pasted-code":     {"%s", "    for x in items:", "\tprint(x)", "@", "  more code", "", "@", "end"},
	"indent-under-at": {"%s", "@", "   indented under a drawn rule", "end"},
}

func piIndentedDraft(t *testing.T, pane, frame string, shape []string, head string) string {
	t.Helper()
	draft := make([]string, 0, len(shape))
	for _, line := range shape {
		switch {
		case line == "@":
			line = frame
		case strings.Contains(line, "%s"):
			line = strings.Replace(line, "%s", head, 1)
		}
		draft = append(draft, line)
	}
	return spliceFramedDraft(t, pane, draft)
}

// R304/R324/R316: in pi, a draft's own indentation is draft, also when the
// draft holds a line drawn exactly like the frame. Editing the draft never
// advances last_output_at, never makes the row unseen and never enables
// movement, whether the clock first meets the session with the draft already
// typed (cold) or sees it typed after a baseline (warm), with and without a
// frame-identical divider in the transcript. A real transcript change above
// the same draft still counts, and two of them in consecutive buckets move
// the row, without touching last_seen_at.
func TestPiIndentedFrameMimickingDraftStaysOutOfTheClock(t *testing.T) {
	goldens := []struct{ golden, line string }{
		{"pi/idle.txt", " Ten."},
		{"pi/running.txt", " run: echo hello-from-tool"},
	}
	for _, g := range goldens {
		golden, frame := framedGolden(t, g.golden)
		bases := map[string]string{
			"plain":   golden,
			"divider": dividerPane(t, golden, frame, []string{"@", " under the divider"}),
		}
		for baseName, base := range bases {
			if !strings.Contains(base, g.line+"\n") {
				t.Fatalf("%s: golden lacks transcript line %q", g.golden, g.line)
			}
			for shapeName, shape := range piIndentedDrafts {
				for _, mode := range []string{"cold", "warm"} {
					t.Run(g.golden+"/"+baseName+"/"+shapeName+"/"+mode, func(t *testing.T) {
						r := newMovementRig(t, "pi", 2*time.Hour)
						typed := func(pane, head string) string { return piIndentedDraft(t, pane, frame, shape, head) }
						if mode == "cold" {
							r.tickPane(1000, 1000, typed(base, "please explain"))
						} else {
							r.tickPane(1000, 1000, base)
						}
						quiet := r.attention()
						for i, head := range []string{"please explain again", "please explain once more", "please explain"} {
							now := int64(1010 + 10*i)
							r.tickPane(now, now, typed(base, head))
							r.assertQuiet(now, quiet)
							if r.clock.Moving(r.id) {
								t.Fatalf("a draft-only edit at %d enabled movement", now)
							}
						}
						once := strings.Replace(base, g.line+"\n", g.line+" More.\n", 1)
						r.tickPane(1040, 1040, typed(once, "please explain"))
						got := r.row()
						if got.LastOutputAt != 1040000 || !got.Unseen() || got.LastSeenAt != quiet.seen {
							t.Fatalf("a transcript change above the draft did not count: last_output_at=%d unseen=%v seen=%d", got.LastOutputAt, got.Unseen(), got.LastSeenAt)
						}
						twice := strings.Replace(base, g.line+"\n", g.line+" More. Most.\n", 1)
						r.tickPane(1050, 1050, typed(twice, "please explain"))
						if !r.clock.Moving(r.id) {
							t.Fatal("two real transcript changes did not enable movement")
						}
						r.wantStatus("running", "transcript changes in two consecutive buckets")
						if got := r.row(); got.LastSeenAt != quiet.seen {
							t.Fatalf("output advanced last_seen_at: %d", got.LastSeenAt)
						}
						for i, head := range []string{"edited after output", "edited again"} {
							now := int64(1060 + 10*i)
							r.tickPane(now, now, typed(twice, head))
							if got := r.row(); got.LastOutputAt != 1050000 {
								t.Fatalf("draft edit after output counted at %d: last_output_at=%d", now, got.LastOutputAt)
							}
						}
					})
				}
			}
		}
	}
}
