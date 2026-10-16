package service

import (
	"strings"
	"testing"
	"time"
)

// R304/R324/R316: pi transcript text under a frame-identical divider is
// transcript at any indentation depth. Whether the clock first meets the pane
// with it (cold) or after a baseline without it (warm), each change to it
// advances last_output_at past last_seen_at, makes the idle row unseen and,
// in consecutive buckets, moves the row, without touching last_seen_at; with a
// draft in the composer, a draft-only edit still never counts. The first tail
// is the shape review found; the others are the same rule in shapes it did
// not list.
func TestPiIndentedTranscriptUnderDividerFeedsTheClock(t *testing.T) {
	tails := map[string][]string{
		"finding-3-cols": {"@", "   code: return verifierValue(%s)"},
		"two-cols":       {"@", "  result: %s"},
		"code-block":     {"@", "   ```go", "     return verifierValue(%s)", "   ```"},
	}
	drafts := map[string][]string{
		"no-draft":  nil,
		"draft":     {"please explain"},
		"mimicking": {"please explain", "  preserve this indentation", "", "@", "appendix"},
	}
	golden, frame := framedGolden(t, "pi/idle.txt")
	for tailName, tail := range tails {
		for draftName, draft := range drafts {
			for _, mode := range []string{"cold", "warm"} {
				t.Run(tailName+"/"+draftName+"/"+mode, func(t *testing.T) {
					pane := func(value, head string) string {
						filled := make([]string, len(tail))
						for i, line := range tail {
							filled[i] = strings.Replace(line, "%s", value, 1)
						}
						p := dividerPane(t, golden, frame, filled)
						if draft == nil {
							return p
						}
						d := make([]string, len(draft))
						for i, line := range draft {
							switch {
							case line == "@":
								line = frame
							case i == 0:
								line = head
							}
							d[i] = line
						}
						return spliceFramedDraft(t, p, d)
					}
					r := newMovementRig(t, "pi", 2*time.Hour)
					if mode == "warm" {
						r.tickPane(990, 990, golden)
					}
					r.tickPane(1000, 1000, pane("41", "please explain"))
					seen := r.row().LastSeenAt
					for i, value := range []string{"42", "43"} {
						now := int64(1010 + 10*i)
						r.tickPane(now, now, pane(value, "please explain"))
						got := r.row()
						if got.LastOutputAt != now*1000 || got.LastSeenAt != seen || got.LastOutputAt <= got.LastSeenAt {
							t.Fatalf("transcript change at %d: last_output_at=%d last_seen_at=%d (seen was %d)", now, got.LastOutputAt, got.LastSeenAt, seen)
						}
						if got.Status == "idle" && !got.Unseen() {
							t.Fatalf("idle row with real output is not unseen: status=%q unseen=%v", got.Status, got.Unseen())
						}
					}
					if !r.clock.Moving(r.id) {
						t.Fatal("two consecutive transcript changes did not enable movement")
					}
					r.wantStatus("running", "transcript changes in two consecutive buckets")
					if draft == nil {
						return
					}
					for i, head := range []string{"edited draft", "edited draft again"} {
						now := int64(1030 + 10*i)
						r.tickPane(now, now, pane("43", head))
						if got := r.row(); got.LastOutputAt != 1020000 || got.LastSeenAt != seen {
							t.Fatalf("a draft-only edit at %d counted: last_output_at=%d last_seen_at=%d", now, got.LastOutputAt, got.LastSeenAt)
						}
					}
				})
			}
		}
	}
}
