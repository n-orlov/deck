package features

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestSessionsCreatedGapWithinBudgetRaceIndicator drives
// sessionsCreatedGapWithinBudget (the comparison behind
// codex_hooks.feature's "created within 2 seconds of each other" step)
// directly with an injected race indicator, proving R164's requirement that
// the 2000ms creation-gap budget is skipped only on a race build and that a
// normal build's comparison is unchanged from dc2b6f7ece: a 2001ms gap still
// fails, a 2000ms gap still passes, and the gap's sign does not matter. Only
// when the injected indicator is true does an over-budget gap pass -- with
// the measured value logged rather than silently dropped.
func TestSessionsCreatedGapWithinBudgetRaceIndicator(t *testing.T) {
	const base = int64(1_700_000_000_000)

	// Normal build (raceBuild=false): dc2b6f7ece's `delta > 2000` check.
	if err := sessionsCreatedGapWithinBudget("one", "two", base, base+2001, false); err == nil {
		t.Fatal("normal build: want failure for a 2001ms creation gap against a 2000ms budget, got nil")
	}
	if err := sessionsCreatedGapWithinBudget("one", "two", base+2001, base, false); err == nil {
		t.Fatal("normal build: want failure for a reversed 2001ms creation gap against a 2000ms budget, got nil")
	}
	if err := sessionsCreatedGapWithinBudget("one", "two", base, base+2000, false); err != nil {
		t.Fatalf("normal build: want success for a 2000ms creation gap against a 2000ms budget: %v", err)
	}
	if err := sessionsCreatedGapWithinBudget("one", "two", base, base+250, false); err != nil {
		t.Fatalf("normal build: want success for a 250ms creation gap: %v", err)
	}

	// Race build (raceBuild=true): the over-budget gap that failed above
	// must pass, and the skip must be logged with the measured gap.
	var logged bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	if err := sessionsCreatedGapWithinBudget("one", "two", base, base+2153, true); err != nil {
		t.Fatalf("race build: want success for a 2153ms creation gap (budget skipped), got: %v", err)
	}
	if !strings.Contains(logged.String(), "2153ms") {
		t.Fatalf("race build: want the logged skip text to contain the measured gap 2153ms, got %q", logged.String())
	}
}
