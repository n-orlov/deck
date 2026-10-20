package tui

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/service"
)

type recordedOutput struct{ at map[string]int64 }

func (r *recordedOutput) RecordOutput(_ context.Context, id string, at int64) error {
	r.at[id] = at
	return nil
}

// R306 end to end: a reload that widens [ui] output_bucket reaches the real
// output clock, which then judges held readings at the new width. A baseline
// at 1700000000 under 2.000000001s and a transcript change at 1700000003
// after widening to 4.000000007s are buckets 424999999 and 425000000, so the
// change writes last_output_at; a second change at 1700000004 is in that same
// bucket and writes nothing.
func TestReloadWideningReachesTheRealOutputClock(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nsort_order = \"activity\"\noutput_bucket = \"2.000000001s\"\n")
	clock := service.NewOutputClock(h.m.settings.OutputBucket)
	h.m = h.m.WithOutputDiscounter(clock)
	store := &recordedOutput{at: map[string]int64{}}
	pane := "base\n"
	observe := func(activity int64) bool {
		t.Helper()
		wrote, err := clock.Observe(context.Background(), store, "s", activity, func(context.Context) (string, error) { return pane, nil })
		if err != nil {
			t.Fatal(err)
		}
		return wrote
	}
	observe(1700000000)
	h.writeConfig("[ui]\nsort_order = \"activity\"\noutput_bucket = \"4.000000007s\"\n")
	h.reload()
	if want := 4*time.Second + 7; h.m.settings.OutputBucket != want {
		t.Fatalf("OutputBucket = %v after reload, want %v", h.m.settings.OutputBucket, want)
	}
	pane = "base\nmore\n"
	if !observe(1700000003) || store.at["s"] != 1700000003*1000 {
		t.Fatalf("widened by reload: change at 1700000003 recorded %d, want %d", store.at["s"], int64(1700000003*1000))
	}
	pane = "base\nmore\nagain\n"
	if observe(1700000004) {
		t.Fatal("widened by reload: a second change in bucket 425000000 wrote again")
	}
}
