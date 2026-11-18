// render_test.go proves PRD phase3b II-27: renders are coalesced at
// renderCoalesceInterval (the DECK_INTERACTIVE_MS knob's package-level
// stand-in, per grid.go's own var doc) rather than one per pipe read, and
// records the measured cost claim -- render frequency, not parsing,
// dominates the transport's cost.
package interactive

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// TestRenderCoalescerNeverFiresWithoutADirtySignal is the non-vacuous
// negative control for every other test in this file: a coalescer that
// fired on every tick regardless of MarkDirty would make every positive
// assertion below meaningless.
func TestRenderCoalescerNeverFiresWithoutADirtySignal(t *testing.T) {
	c := NewRenderCoalescer(10 * time.Millisecond)
	defer c.Close()

	select {
	case <-c.Renders():
		t.Fatalf("render fired with MarkDirty never called")
	case <-time.After(60 * time.Millisecond):
	}
}

// TestRenderCoalescerFiresAtMostOncePerIntervalRegardlessOfDirtyCallCount
// is the core II-27 claim at the RenderCoalescer level, deterministic and
// independent of tmux/pipe timing: many MarkDirty calls close together
// collapse into a render count bounded by elapsed-time/interval, never
// one render per call.
func TestRenderCoalescerFiresAtMostOncePerIntervalRegardlessOfDirtyCallCount(t *testing.T) {
	c := NewRenderCoalescer(20 * time.Millisecond)
	defer c.Close()

	const calls = 50
	start := time.Now()
	for i := 0; i < calls; i++ {
		c.MarkDirty()
		time.Sleep(1 * time.Millisecond)
	}
	elapsed := time.Since(start)

	count := 0
	deadline := time.After(60 * time.Millisecond)
loop:
	for {
		select {
		case <-c.Renders():
			count++
		case <-deadline:
			break loop
		}
	}

	t.Logf("%d MarkDirty calls over %v at a 20ms interval produced %d renders", calls, elapsed, count)
	if count == 0 {
		t.Fatalf("no renders fired despite %d MarkDirty calls over %v -- the coalescer is not delivering at all", calls, elapsed)
	}
	if count >= calls/2 {
		t.Fatalf("got %d renders for %d MarkDirty calls -- expected a small number bounded by roughly elapsed/interval, not one render per call (that would be the un-coalesced baseline II-27 replaces)", count, calls)
	}
}

// TestRenderCoalescerAtTwoIntervalSettingsGivesDifferentRenderCounts is
// II-27's successCriteria taken literally: the SAME dirty-signal pattern
// driven through the knob at two settings must produce different render
// counts, proving the knob actually controls the coalescing rate rather
// than being decorative.
func TestRenderCoalescerAtTwoIntervalSettingsGivesDifferentRenderCounts(t *testing.T) {
	const driveFor = 200 * time.Millisecond
	drive := func(interval time.Duration) int {
		c := NewRenderCoalescer(interval)
		defer c.Close()

		// MarkDirty must be driven CONCURRENTLY with counting, not before
		// it: Renders() is buffered exactly 1 (deliberately, see its own
		// doc), so a driving loop that runs to completion before anyone
		// starts draining would lose every notification but the single
		// most recent one -- that is the coalescing this type is FOR, but
		// it would make this test measure "how many renders survive being
		// entirely unconsumed", not "how many renders a live consumer
		// sees", which is the property II-27 actually cares about.
		go func() {
			stop := time.Now().Add(driveFor)
			for time.Now().Before(stop) {
				c.MarkDirty()
				time.Sleep(2 * time.Millisecond)
			}
		}()

		count := 0
		deadline := time.After(driveFor + interval + 80*time.Millisecond)
		for {
			select {
			case <-c.Renders():
				count++
			case <-deadline:
				return count
			}
		}
	}

	small := drive(10 * time.Millisecond)
	large := drive(100 * time.Millisecond)
	t.Logf("~200ms of MarkDirty calls (2ms apart) at a 10ms interval: %d renders; at a 100ms interval: %d renders", small, large)

	if large >= small {
		t.Fatalf("coalescing to the LONGER interval (100ms, %d renders) did not produce fewer renders than the shorter one (10ms, %d renders) -- the knob has no observed effect", large, small)
	}
	if small < 8 {
		t.Fatalf("the 10ms interval only produced %d renders against ~200ms of continuous dirty signals -- suspiciously few, the driving pattern may not have actually run", small)
	}
	if large > 5 {
		t.Fatalf("the 100ms interval produced %d renders against a ~280ms counting window -- expected roughly 280/100 ~= 2-3, coalescing is not bounding the rate to the interval", large)
	}
}

// countAndTimeRenders drains a Session's Renders() channel for dur,
// calling Grid().Render() on every notification (the shape Renders()'s
// own doc names: the channel carries no frame data, a caller renders by
// calling Grid().Render() when told a repaint is due) and returning how
// many notifications arrived plus the wall-clock cost of each Render()
// call, for the cost-ratio measurement below.
func countAndTimeRenders(s *Session, dur time.Duration) (count int, samples []time.Duration) {
	deadline := time.After(dur)
	for {
		select {
		case <-s.Renders():
			start := time.Now()
			_ = s.Grid().Render()
			samples = append(samples, time.Since(start))
			count++
		case <-deadline:
			return count, samples
		}
	}
}

// medianDuration is the middle sample, so one descheduled Render() call
// (a GC pause, a CPU-starved agent) cannot move the per-call cost the way
// it moves a sum of a handful of ~10us calls.
func medianDuration(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// modelRenderCost is II-27's cost claim made explicit: the transport's
// render cost at a setting is how many renders it ran times what one
// Render() costs, so frequency is the lever. The per-call cost is measured
// (pooled median of every Render() call of the three runs), not assumed;
// only the product is taken, because a raw wall-clock sum over a ~10-render
// run is dominated by scheduling noise (build 69 measured 524us for 42
// renders against 583us for 10), which says nothing about the claim.
func modelRenderCost(count int, perCall time.Duration) time.Duration {
	return time.Duration(count) * perCall
}

// TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern is II-27's
// successCriteria against the real transport: a real tmux pane runs a
// throttled, known byte-arrival pattern (40 lines, ~12ms apart, ~480ms
// total -- the same throttled-loop idiom task 043's own gotcha
// establishes as reproducible here, in contrast to an unthrottled flood),
// and Session.Renders() is counted at three settings of the
// renderCoalesceInterval knob: effectively-per-read (1ms, bounded only by
// ticker granularity), the SPEC-default coalesced interval (60ms), and an
// interval longer than the whole counting window (2s), which is this
// test's own non-vacuous control -- the ticker literally gates on the
// interval, not on the pattern, so it must produce zero renders.
func TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern(t *testing.T) {
	const lines = 40
	const lineInterval = 12 * time.Millisecond
	const countWindow = 900 * time.Millisecond

	runPattern := func(t *testing.T, interval time.Duration) (count int, samples []time.Duration) {
		socket := interactiveSocket("render-coalesce")
		cleanup := newBareInteractiveSession(t, socket, 40, 10)
		defer cleanup()
		client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
		ctx := context.Background()

		original := renderCoalesceInterval
		renderCoalesceInterval = interval
		defer func() { renderCoalesceInterval = original }()

		session, err := Start(ctx, client, "s0", 40, 10, func(_ context.Context) ([]byte, error) {
			return rawCapturePane(t, socket, "s0"), nil
		})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		defer session.Close()

		// The known pattern: `lines` echoes, `lineInterval` apart, run
		// inside the pane's own shell -- a real, reproducible
		// byte-arrival cadence, not a synthetic in-process signal.
		sendLiteralLine(t, socket, "s0", "i=0; while [ $i -lt 40 ]; do echo out-$i; sleep 0.012; i=$((i+1)); done")

		return countAndTimeRenders(session, countWindow)
	}

	perRead, perReadSamples := runPattern(t, 1*time.Millisecond)
	coalesced60, coalesced60Samples := runPattern(t, 60*time.Millisecond)
	longerThanWindow, longerThanWindowSamples := runPattern(t, 2*time.Second)

	var pooled []time.Duration
	pooled = append(pooled, perReadSamples...)
	pooled = append(pooled, coalesced60Samples...)
	pooled = append(pooled, longerThanWindowSamples...)
	perCall := medianDuration(pooled)
	perReadCost := modelRenderCost(perRead, perCall)
	coalesced60Cost := modelRenderCost(coalesced60, perCall)
	longerThanWindowCost := modelRenderCost(longerThanWindow, perCall)

	t.Logf("known pattern (%d lines, %v apart, ~%v total), counted over %v:", lines, lineInterval, time.Duration(lines)*lineInterval, countWindow)
	t.Logf("  interval=1ms   (effectively per-read): %d renders x %v median Render() = %v modelled cost", perRead, perCall, perReadCost)
	t.Logf("  interval=60ms  (SPEC default, coalesced): %d renders x %v median Render() = %v modelled cost", coalesced60, perCall, coalesced60Cost)
	t.Logf("  interval=2s    (longer than the counting window, non-vacuous control): %d renders x %v median Render() = %v modelled cost", longerThanWindow, perCall, longerThanWindowCost)
	if coalesced60Cost > 0 {
		t.Logf("  measured ratio, per-read Render() cost / 60ms-coalesced Render() cost: %.2fx (PRD II-27 cites 1.99x from spike evidence unreachable in this container, per tasks.json's discovered.prdCorrections; this is this repository's own reproducible measurement of the same property, not a reproduction of that exact figure)", float64(perReadCost)/float64(coalesced60Cost))
	}

	if longerThanWindow != 0 {
		t.Fatalf("an interval (2s) longer than the whole %v counting window produced %d renders, not 0 -- the coalescer is not actually gating on the interval, or this control is vacuous", countWindow, longerThanWindow)
	}
	if coalesced60 >= perRead {
		t.Fatalf("coalescing to 60ms (%d renders) did not produce fewer renders than the effectively-per-read setting (%d renders) against the identical known pattern", coalesced60, perRead)
	}
	if perRead < 15 {
		t.Fatalf("the effectively-per-read setting only produced %d renders against a %d-line known pattern -- suspiciously few; the pattern may not have actually run against the real pane", perRead, lines)
	}
	if coalesced60 > 15 {
		t.Fatalf("coalescing to 60ms produced %d renders against a %v window -- expected roughly %v/60ms (~%d), not a count close to the %d-line pattern itself", coalesced60, countWindow, countWindow, int(countWindow/(60*time.Millisecond))+2, lines)
	}
	if perReadCost <= coalesced60Cost {
		t.Fatalf("modelled Render() cost at the effectively-per-read setting (%v) is not greater than at 60ms coalescing (%v) -- II-27's claim that render frequency dominates the transport's cost does not hold for this measurement", perReadCost, coalesced60Cost)
	}
}

// TestRenderCostModelRanksByRenderCountDespiteNoisyPerCallSamples pins the
// measurement rule the test above relies on, on the exact shape build 69
// hit: the per-read run's calls were individually cheaper than the
// coalesced run's (12us against 58us averages), which made a raw
// wall-clock sum rank the 42-render run below the 10-render run.
func TestRenderCostModelRanksByRenderCountDespiteNoisyPerCallSamples(t *testing.T) {
	us := time.Microsecond
	perRead := make([]time.Duration, 42)
	for i := range perRead {
		perRead[i] = 12 * us
	}
	coalesced := []time.Duration{400 * us, 40 * us, 30 * us, 20 * us, 20 * us, 20 * us, 20 * us, 20 * us, 20 * us, 20 * us}
	rawPerRead, rawCoalesced := sumDurations(perRead), sumDurations(coalesced)
	if rawPerRead >= rawCoalesced {
		t.Fatalf("fixture lost its point: raw sums %v vs %v no longer invert the ranking", rawPerRead, rawCoalesced)
	}
	pooled := append(append([]time.Duration(nil), perRead...), coalesced...)
	unit := medianDuration(pooled)
	if got, want := modelRenderCost(42, unit), modelRenderCost(10, unit); got <= want {
		t.Fatalf("modelled cost of 42 renders (%v) is not above that of 10 renders (%v)", got, want)
	}
}

// TestRenderCostModelDoesNotInventACostGap covers the other direction: with
// equal render counts the model yields equal costs, and with no renders
// (the 2s control) it yields zero, so the "greater than" assertion above
// cannot be satisfied by an un-coalesced run.
func TestRenderCostModelDoesNotInventACostGap(t *testing.T) {
	unit := medianDuration([]time.Duration{5 * time.Microsecond, 9 * time.Microsecond, 400 * time.Microsecond})
	if unit != 9*time.Microsecond {
		t.Fatalf("median = %v, want 9us (an outlier must not move it)", unit)
	}
	if a, b := modelRenderCost(10, unit), modelRenderCost(10, unit); a != b {
		t.Fatalf("equal counts modelled unequal costs %v and %v", a, b)
	}
	if got := modelRenderCost(0, unit); got != 0 {
		t.Fatalf("zero renders modelled cost %v, want 0", got)
	}
	if got := medianDuration(nil); got != 0 {
		t.Fatalf("median of no samples = %v, want 0", got)
	}
}

func sumDurations(ds []time.Duration) (total time.Duration) {
	for _, d := range ds {
		total += d
	}
	return total
}

// TestRenderCostPerCallDominatesParsingCostPerCall is the other half of
// II-27's report claim ("render frequency, not parsing, dominates the
// transport's cost"): Render() composing the whole grid into a string is
// measured directly against Write() parsing one chunk of realistic
// program output, on the same 120x40 grid docs/reports/phase3b.md's own
// ~53MiB baseline uses, with neither side favoured by a synthetic
// worst/best case -- the SAME chunk is used for every Write() call, and
// Render() is called against the grid's actual accumulated content.
func TestRenderCostPerCallDominatesParsingCostPerCall(t *testing.T) {
	g := newGrid(120, 40)

	// A representative chunk: plain text, one SGR colour transition, a
	// reset, and a line ending -- not chosen to favour either side.
	chunk := []byte("\x1b[32mok\x1b[0m: processed item number 12345 of 99999 in batch\r\n")

	const n = 2000

	writeStart := time.Now()
	for i := 0; i < n; i++ {
		if _, err := g.Write(chunk); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	writeElapsed := time.Since(writeStart)

	renderStart := time.Now()
	for i := 0; i < n; i++ {
		_ = g.Render()
	}
	renderElapsed := time.Since(renderStart)

	t.Logf("%d Write() calls: %v total (%v/call); %d Render() calls: %v total (%v/call); Render()/Write() per-call cost ratio: %.2fx",
		n, writeElapsed, writeElapsed/n, n, renderElapsed, renderElapsed/n, float64(renderElapsed)/float64(writeElapsed))

	if renderElapsed <= writeElapsed {
		t.Fatalf("Render() cost (%v for %d calls) is not greater than Write() cost (%v for %d calls) -- II-27's claim that render frequency, not parsing, dominates the transport's cost does not hold for this measurement", renderElapsed, n, writeElapsed, n)
	}
}
