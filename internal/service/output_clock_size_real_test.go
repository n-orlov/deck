package service

import (
	"strings"
	"testing"
	"time"
)

// outputPaneFooter is the footer the real-output tests draw.
const outputPaneFooter = "~/proj  main  ctx:12%  opus"

// claudePaneWithOutput is claudePane with one more transcript line printed
// above the frame: real agent output, as opposed to a reflow or footer churn.
func claudePaneWithOutput(width int) string {
	pane := claudePane(width, outputPaneFooter)
	at := strings.Index(pane, "\n─")
	return pane[:at] + "\n● Finished: the reconcile loop keeps its baseline" + pane[at:]
}

// TestOutputClockSizeChangeStillCountsRealOutput is R409's cases 4 and 5: the
// size-keyed baseline must not swallow real output. Case 4 resizes (tick 2, a
// baseline at the new size) and then prints real transcript output in a later
// bucket at that size; case 5 prints real output with no resize (the control).
// Each row drives the real cut and hash (captureTranscriptReadings) over the
// claude-style pane and asserts exactly one last_output_at write, at the second
// of the bucket the output was seen in; a later tick showing the same text
// writes nothing more.
func TestOutputClockSizeChangeStillCountsRealOutput(t *testing.T) {
	const (
		wide, narrow = "80|24", "60|24"
		footer       = "~/proj  main  ctx:12%  opus"
	)
	rows := []struct {
		name         string
		resizeTo     string // the size tick 2 reads; the baseline is wide
		drawnAt      int    // the width the pane is drawn at from tick 2 on
		tick2Seconds string
	}{
		{"4 resize, then real transcript output in a later bucket at the new size", narrow, 60, "2003"},
		{"5 real output with no resize", wide, 80, "2003"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newSizeRig(t)
			r.tick("2000", wide, claudePane(80, footer), "")
			// Tick 2: the resize (or nothing) with the transcript unchanged.
			r.tick(row.tick2Seconds, row.resizeTo, claudePane(row.drawnAt, footer), "")
			if got := r.lastOutputAt(); got != 0 {
				t.Fatalf("tick 2 wrote last_output_at = %d; a resize alone is never output", got)
			}
			// Tick 3: real output in a later bucket at the settled size.
			r.tick("2025", row.resizeTo, claudePaneWithOutput(row.drawnAt), "")
			if got, want := r.lastOutputAt(), int64(2025_000); got != want {
				t.Fatalf("real output wrote last_output_at = %d, want exactly one write at %d", got, want)
			}
			// Tick 4: a later bucket, the same text: nothing more is written.
			r.tick("2037", row.resizeTo, claudePaneWithOutput(row.drawnAt), "")
			if got, want := r.lastOutputAt(), int64(2025_000); got != want {
				t.Fatalf("an unchanged transcript wrote last_output_at = %d, want it to stay %d", got, want)
			}
		})
	}
}

// TestMovementFallbackIgnoresResizeReflow is R409's case 8: the movement
// fallback (R316) needs counted buckets in two consecutive buckets, and a
// resize's reflow is never a counted bucket, so cases 1, 2 and 3 never reach
// `running`. Each row is a baseline at 80x24 (activity 2000), then the case's
// tick 2 (a reflow at 60x24 with window_activity unmoved, moved in the same
// bucket, or moved into a new bucket) and then two more resizes in
// consecutive buckets (50x24 at 2021, 40x24 at 2031) with the clock at 2032:
// where a reflow counted, those would be the two consecutive counted buckets.
// After every tick the clock does not report Moving, no bucket was counted
// (last_output_at is unwritten) and the row is still `idle`, reconcile's
// applyMovement included. The last test proves this rig reaches `running` on
// real output in two consecutive buckets, so the `idle` above means something.
func TestMovementFallbackIgnoresResizeReflow(t *testing.T) {
	const footer = "~/proj  main  ctx:12%  opus"
	rows := []struct {
		name  string
		tick2 string // window_activity of the first reflow tick
	}{
		{"1 resize with a reflow and no window_activity move", "2000"},
		{"2 resize with a reflow and window_activity moved in the same bucket", "2003"},
		{"3 resize with a reflow and window_activity moved into a new bucket", "2012"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newSizeRigAt(t, 90*time.Minute)
			now := time.Unix(2032, 0)
			r.svc.Output.Now = func() time.Time { return now }
			r.tick("2000", "80|24", claudePane(80, footer), "")
			steps := []struct {
				activity, size string
				width          int
			}{{row.tick2, "60|24", 60}, {"2021", "50|24", 50}, {"2031", "40|24", 40}}
			for i, step := range steps {
				r.tick(step.activity, step.size, claudePane(step.width, footer), "")
				got, err := r.db.GetSession(t.Context(), r.id)
				if err != nil {
					t.Fatal(err)
				}
				if got.LastOutputAt != 0 {
					t.Fatalf("reflow %d counted a bucket: last_output_at = %d", i+1, got.LastOutputAt)
				}
				if r.svc.Output.Moving(r.id) {
					t.Fatalf("reflow %d: the output clock reports Moving", i+1)
				}
				if got.Status == "running" || got.StatusSource == "movement" {
					t.Fatalf("reflow %d: applyMovement made the row %s (%s)", i+1, got.Status, got.StatusSource)
				}
			}
		})
	}
	t.Run("control: real output in two consecutive buckets reaches running", func(t *testing.T) {
		r := newSizeRigAt(t, 90*time.Minute)
		r.svc.Output.Now = func() time.Time { return time.Unix(2032, 0) }
		r.tick("2000", "80|24", claudePane(80, footer), "")
		r.tick("2021", "80|24", claudePaneWithOutput(80), "")
		more := strings.Replace(claudePaneWithOutput(80), "\n─", "\n● And one more line of output\n─", 1)
		r.tick("2031", "80|24", more, "")
		got, err := r.db.GetSession(t.Context(), r.id)
		if err != nil {
			t.Fatal(err)
		}
		if !r.svc.Output.Moving(r.id) || got.Status != "running" || got.StatusSource != "movement" {
			t.Fatalf("real output in consecutive buckets: Moving = %v, status %s (%s); the rig cannot reach running",
				r.svc.Output.Moving(r.id), got.Status, got.StatusSource)
		}
	})
}
