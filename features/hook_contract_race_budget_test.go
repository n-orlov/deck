package features

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestHookStoreDurationWithinBudgetRaceIndicator drives
// hookStoreDurationWithinBudget directly with an injected race indicator,
// proving R164's requirement that the 20ms hook-store-duration budget is
// skipped only on a race build, and that a normal build's comparison is
// unchanged from dc2b6f7ece: 20.153ms still fails a 20ms budget, 19ms still
// passes it, and only when the injected indicator is true does 20.153ms
// pass -- with the measured value logged rather than silently dropped.
func TestHookStoreDurationWithinBudgetRaceIndicator(t *testing.T) {
	// Normal build (raceBuild=false): the limit and the `< limit`
	// comparison are exactly dc2b6f7ece's -- 20.153ms overshoots a 20ms
	// budget and must fail.
	if err := hookStoreDurationWithinBudget(20.153, 20, false); err == nil {
		t.Fatal("normal build: want failure for 20.153ms against a 20ms budget, got nil")
	}

	// Normal build, comfortably under budget: 19ms must still pass.
	if err := hookStoreDurationWithinBudget(19, 20, false); err != nil {
		t.Fatalf("normal build: want success for 19ms against a 20ms budget: %v", err)
	}

	// Race build (raceBuild=true): the same 20.153ms that failed above
	// must now pass, because the budget assertion is skipped on a race
	// build, and the skip must be logged with the measured value so the
	// scenario stays informative rather than silently blind.
	var logged bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	if err := hookStoreDurationWithinBudget(20.153, 20, true); err != nil {
		t.Fatalf("race build: want success for 20.153ms against a 20ms budget (skipped), got: %v", err)
	}
	if !strings.Contains(logged.String(), "20.153") {
		t.Fatalf("race build: want the logged skip text to contain the measured value 20.153, got %q", logged.String())
	}
}
