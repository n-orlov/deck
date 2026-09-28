package features

import (
	"os"
	"path/filepath"
	"testing"
)

// TestR150GuardRejectsTransientAndUnsettledShellWaypoints (cure-01-02, R150)
// pins the three defects review found in the shipped
// scanFeatureFileForUnsettledFrameCapture (see
// TestR150EveryShellFrameCaptureIsSettled's own doc comment): waiting on the
// transient "starting" status word was wrongly treated as a settle signal,
// a capture with no preceding wait at all was never even inspected, and a
// generic "running" wait was trusted to settle the sole remaining
// unconfirmed shell even when another shell's already-"running" text could
// have satisfied it by itself. Each fixture below is a minimal
// *.feature-shaped scenario reproducing exactly one of those admitted
// classes (plus mouse.feature's own historical pre-fix scenario, task 004's
// commit 04847650c6, as the fifth, concrete, non-synthetic case) and must
// come back with at least one violation from scanFeatureFileForUnsettledFrameCapture.
func TestR150GuardRejectsTransientAndUnsettledShellWaypoints(t *testing.T) {
	cases := []struct {
		name    string
		feature string
	}{
		{
			name: "starting_wait_before_capture",
			// Bug 1: a wait that only ever names the TRANSIENT status word
			// must never mark the shell it names settled -- a capture right
			// after it is exactly as unsettled as one with no wait at all.
			feature: `Feature: fixture
  Scenario: starting wait before capture
    Given deck client "A" is started
    When deck client "A" creates shell session "transient-only"
    And deck client "A" screen contains "starting"
    And deck client "A" captures its frame as "transient-frame"
    Then deck client "A" frame still matches the captured "transient-frame" frame
`,
		},
		{
			name: "starting_wait_then_capture_compare",
			// Bug 1, restated with the wait and the capture on adjacent
			// steps and the compare immediately following, matching the
			// reviewer's own "that same wait followed by capture/compare"
			// fixture description.
			feature: `Feature: fixture
  Scenario: starting wait then capture and compare
    Given deck client "A" is started
    When deck client "A" creates shell session "transient-adjacent"
    And within one configured reconcile interval deck client "A" screen contains "starting"
    And deck client "A" captures its frame as "transient-adjacent-frame"
    And deck client "A" sends "x"
    Then deck client "A" frame still matches the captured "transient-adjacent-frame" frame
`,
		},
		{
			name: "no_wait_capture",
			// Bug 2: a capture with NO preceding wait of any kind must
			// still be inspected -- the shipped scan only ever looked once
			// some generic wait had been seen at all, so a capture that
			// never triggered that gate slipped through untested.
			feature: `Feature: fixture
  Scenario: capture with no wait at all
    Given deck client "A" is started
    When deck client "A" creates shell session "no-wait-target"
    And deck client "A" captures its frame as "no-wait-frame"
    Then deck client "A" frame still matches the captured "no-wait-frame" frame
`,
		},
		{
			name: "already_settled_other_row",
			// Bug 3: alpha is settled BY NAME first; bravo is created
			// afterward and left to a generic "running" wait, which alpha's
			// own still-"running" text can satisfy on its own, proving
			// nothing about bravo, which is exactly what the shipped scan's
			// "exactly one unsettled shell" shortcut wrongly trusted.
			feature: `Feature: fixture
  Scenario: a generic wait cannot settle bravo merely because alpha is already settled
    Given deck client "A" is started
    When deck client "A" creates shell session "settle-order-alpha"
    And within one configured reconcile interval deck client "A" row "settle-order-alpha" contains "running"
    And deck client "A" creates shell session "settle-order-bravo"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "settle-order-frame"
    Then deck client "A" frame still matches the captured "settle-order-frame" frame
`,
		},
		{
			name: "mouse_feature_historical_unfixed_example",
			// The real defect this whole guard exists for: mouse.feature's
			// own click-enter-alpha/click-enter-bravo scenario BEFORE task
			// 004 fixed it (commit 04847650c6's own diff), reproduced
			// verbatim here as the fixture -- not the current, already-
			// fixed file. Two shells, one generic "running" wait
			// (ambiguous: two unsettled, satisfied by whichever promotes
			// first), then a capture and a later byte-exact compare.
			feature: `Feature: fixture
  @requirement-33-click-selects-and-enters-interactive-mode
  Scenario: a single click on a sidebar row selects it and enters interactive mode on the same press, and Ctrl+Q returns to the list
    Given deck client "A" is started
    When deck client "A" creates shell session "click-enter-alpha"
    And deck client "A" creates shell session "click-enter-bravo"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" clicks on the row containing "click-enter-bravo"
    Then deck client "A" has session "click-enter-bravo" selected
    And deck client "A" screen contains "click-enter-bravo"
    And deck client "A" screen contains "interactive"
    And deck client "A" screen contains "Ctrl+Q"
    And deck client "A" captures its frame as "click-entered-bravo"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" has session "click-enter-bravo" selected
    When deck client "A" enters interactive mode
    Then deck client "A" frame still matches the captured "click-entered-bravo" frame
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeR150Fixture(t, tc.feature)
			violations, err := scanFeatureFileForUnsettledFrameCapture(path)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(violations) == 0 {
				t.Fatalf("R150 guard accepts a transient/unsettled shell-frame waypoint it must reject:\n%s", tc.feature)
			}
		})
	}
}

// TestR150GuardAcceptsValidSettlesAndDurableTransientAssertions is
// TestR150GuardRejectsTransientAndUnsettledShellWaypoints's negative
// counterpart: none of the fix's three corrections may turn a genuinely
// settled or genuinely screen-independent scenario into a false positive.
func TestR150GuardAcceptsValidSettlesAndDurableTransientAssertions(t *testing.T) {
	cases := []struct {
		name    string
		feature string
	}{
		{
			name: "valid_named_settles_for_every_created_shell",
			// The fixed idiom task 004 introduced: every shell created is
			// settled BY NAME before the capture, exactly like mouse.feature's
			// click-enter scenario today.
			feature: `Feature: fixture
  Scenario: both rows settled by name before the capture
    Given deck client "A" is started
    When deck client "A" creates shell session "named-alpha"
    And deck client "A" creates shell session "named-bravo"
    And within one configured reconcile interval deck client "A" row "named-alpha" contains "running"
    And within one configured reconcile interval deck client "A" row "named-bravo" contains "running"
    And deck client "A" captures its frame as "named-frame"
    Then deck client "A" frame still matches the captured "named-frame" frame
`,
		},
		{
			name: "durable_store_only_transient_assertion_is_not_a_screen_wait",
			// "the state database session ... is 'starting' from 'tmux'"
			// (features/status_recovery.feature's own idiom) asserts
			// against the STORE directly -- it names no shell through the
			// "creates shell session" step at all, so it must never feed
			// the created/settled tracking or manufacture a violation
			// merely because the literal word "starting" appears on the
			// line.
			feature: `Feature: fixture
  Scenario: a durable store-only transient assertion never touches the screen
    Given deck client "A" is started
    And the state database session "durable-transient" is "starting" from "tmux" with killed_by_user=0
    And deck client "A" captures its frame as "durable-frame"
    Then deck client "A" frame still matches the captured "durable-frame" frame
`,
		},
		{
			name: "single_shell_generic_running_wait_is_unambiguous",
			// A generic "running" wait genuinely is unambiguous, and so may
			// still settle its shell, when it is the scenario's ONLY shell
			// and no other row is already known-settled (mouse.feature's
			// own preview-wheel-no-op scenario relies on exactly this).
			feature: `Feature: fixture
  Scenario: the only shell created settles the generic wait unambiguously
    Given deck client "A" is started
    When deck client "A" creates shell session "solo-shell"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "solo-frame"
    Then deck client "A" frame still matches the captured "solo-frame" frame
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeR150Fixture(t, tc.feature)
			violations, err := scanFeatureFileForUnsettledFrameCapture(path)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("R150 guard rejects a valid settle/durable assertion it must accept: %v\nfixture:\n%s", violations, tc.feature)
			}
		})
	}
}

// writeR150Fixture writes content to a fresh *.feature file under a t.TempDir
// and returns its path -- scanFeatureFileForUnsettledFrameCapture only reads
// the path given to it, so this needs no godog registration or harness.
func writeR150Fixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.feature")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
