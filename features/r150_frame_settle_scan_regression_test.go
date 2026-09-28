package features

import (
	"os"
	"path/filepath"
	"strings"
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
// cure-01-01-2 adds the ambiguous-settle class: a generic "running" wait
// satisfiable by an already-running agent row or by the preview panel's
// own text (so no generic wait settles a shell any more), and row-scoped
// waits whose row text can be satisfied by a name rather than a status.
func TestR150GuardRejectsTransientAndUnsettledShellWaypoints(t *testing.T) {
	cases := []struct {
		name    string
		feature string
	}{
		{
			name: "reviewer_starting_frame_wait_alone",
			// Review's first fixture, verbatim: a standalone shell-frame
			// wait on the transient "starting" word with NO later capture
			// or compare at all. The wait itself is the flaky waypoint --
			// the shell promotes to "running" within one reconcile tick, so
			// the wait races the promotion -- and must be rejected on its
			// own, not only once some later comparison is seen.
			feature: `Feature: probe
  Scenario: regression
    When deck client "A" creates shell session "alpha"
    Then deck client "A" screen contains "starting"
`,
		},
		{
			name: "reviewer_starting_capture",
			feature: `Feature: probe
  Scenario: regression
    When deck client "A" creates shell session "alpha"
    Then deck client "A" screen contains "starting"
    And deck client "A" captures its frame as "before"
    Then deck client "A" frame still matches the captured "before" frame
`,
		},
		{
			name: "reviewer_no_wait_capture",
			feature: `Feature: probe
  Scenario: regression
    When deck client "A" creates shell session "alpha"
    And deck client "A" captures its frame as "before"
    Then deck client "A" frame still matches the captured "before" frame
`,
		},
		{
			name: "reviewer_already_settled_other_row",
			feature: `Feature: probe
  Scenario: regression
    When deck client "A" creates shell session "alpha"
    Then within one configured reconcile interval deck client "A" row "alpha" contains "running"
    When deck client "A" creates shell session "bravo"
    Then deck client "A" screen contains "running"
    And deck client "A" captures its frame as "before"
    Then deck client "A" frame still matches the captured "before" frame
`,
		},
		{
			name: "row_scoped_starting_wait_on_a_shell",
			// A row-scoped wait naming a SHELL on "starting" is the same
			// transient waypoint, whatever else the screen shows.
			feature: `Feature: fixture
  Scenario: row-scoped starting wait on a shell
    When deck client "A" creates shell session "row-transient"
    Then within one configured reconcile interval deck client "A" row "row-transient" contains "starting"
`,
		},
		{
			name: "background_shell_left_unsettled_before_starting_wait",
			// godog runs Background before every scenario, so a shell the
			// Background creates is displayed in every scenario too:
			// status_theme.feature's starting-token scenario had exactly
			// this shape (tok-anchor settled the generic wait, tok-target
			// never settled by name) until this task settled it.
			feature: `Feature: fixture
  Background:
    Given deck client "A" is started
    When deck client "A" creates shell session "bg-anchor"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    When deck client "A" creates shell session "bg-target"
    And deck client "A" creates claude session "bg-agent" with permission profile "safe"

  Scenario: generic starting wait while a background shell is unconfirmed
    When the state database session "bg-agent" has status "starting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "starting"
`,
		},
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
		{
			name: "running_agent_beside_new_shell",
			// cure-01-01-2 (review): an already-running claude AGENT session
			// -- confirmed running via a generic wait, before any shell
			// exists -- sits beside a brand-new shell. The second generic
			// "running" wait's pass proves nothing about the new shell's own
			// starting->running promotion: it can be (and, on the shipped
			// pre-fix scanner, was) satisfied by the agent's own still-
			// "running" text alone. The shell must still be flagged
			// unsettled at the capture that follows.
			feature: `Feature: fixture
  Scenario: a generic running wait cannot settle a new shell while an agent session is already running
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "mixed-running-agent" with permission profile "safe"
    And within one configured reconcile interval deck client "A" screen contains "running"
    When deck client "A" creates shell session "mixed-running-agent-new-shell"
    And deck client "A" screen contains "running"
    And deck client "A" captures its frame as "mixed-running-frame"
    Then deck client "A" frame still matches the captured "mixed-running-frame" frame
`,
		},
		{
			name: "preview_may_satisfy_generic_running_without_shell_row",
			// cure-01-01-2 (verifier): the shell's own pane prints
			// "running", which the preview panel renders for the selected
			// shell whatever its status. A generic "running" wait then
			// passes on the preview's text alone, proving nothing about the
			// shell's own row -- so it must never settle the shell, even
			// when the shell is the only session on screen.
			feature: `Feature: fixture
  Scenario: preview text satisfies a generic running wait
    Given deck client "A" is started
    When deck client "A" creates shell session "preview-shell"
    And the private tmux pane for session "preview-shell" prints "running"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "preview-frame"
    Then deck client "A" frame still matches the captured "preview-frame" frame
`,
		},
		{
			name: "sole_shell_generic_running_wait",
			// The same generic wait with no explicit print: the preview
			// shows the shell's live pane, whose output the scenario does
			// not control, so the sole-shell generic shortcut is rejected
			// too (it was an accepted control before cure-01-01-2).
			feature: `Feature: fixture
  Scenario: the only shell created, settled only by a generic wait
    Given deck client "A" is started
    When deck client "A" creates shell session "solo-generic"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "solo-generic-frame"
    Then deck client "A" frame still matches the captured "solo-generic-frame" frame
`,
		},
		{
			name: "generic_running_wait_before_any_agent_session",
			// The former "shell created before any agent" control: a
			// generic wait with no other session yet displayed is still
			// satisfiable by the shell's own preview, so it is rejected.
			feature: `Feature: fixture
  Scenario: a generic wait before any agent session exists
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates shell session "early-shell"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" creates claude session "early-later-agent" with permission profile "safe"
    And deck client "A" captures its frame as "early-shell-frame"
    Then deck client "A" frame still matches the captured "early-shell-frame" frame
`,
		},
		{
			name: "row_settle_on_a_name_containing_running",
			// A row-scoped wait whose shell name itself contains "running"
			// is satisfied by the row's name alone, while the row still
			// shows "starting" -- not a session-specific settle.
			feature: `Feature: fixture
  Scenario: the shell's own name satisfies its row wait
    Given deck client "A" is started
    When deck client "A" creates shell session "running-name"
    And within one configured reconcile interval deck client "A" row "running-name" contains "running"
    And deck client "A" captures its frame as "running-name-frame"
    Then deck client "A" frame still matches the captured "running-name-frame" frame
`,
		},
		{
			name: "row_settle_name_is_contained_in_another_running_row",
			// Row "alpha"'s wait also matches the already-running row
			// "alpha-agent", so it proves nothing about shell "alpha".
			feature: `Feature: fixture
  Scenario: another row's name contains the shell's name
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "alpha-agent" with permission profile "safe"
    And within one configured reconcile interval deck client "A" row "alpha-agent" contains "running"
    When deck client "A" creates shell session "alpha"
    And within one configured reconcile interval deck client "A" row "alpha" contains "running"
    And deck client "A" captures its frame as "alpha-frame"
    Then deck client "A" frame still matches the captured "alpha-frame" frame
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
			name: "agent_only_starting_wait_is_durable",
			// An agent session's "starting" is durable (it holds until the
			// agent's own hook reports), so a screen wait on it with no
			// shell displayed is not a transient shell waypoint.
			feature: `Feature: fixture
  Scenario: an agent's starting word is not a shell waypoint
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "agent-only" with permission profile "safe"
    Then deck client "A" screen contains "starting"
    And within one configured reconcile interval deck client "A" row "agent-only" contains "starting"
`,
		},
		{
			name: "starting_wait_after_every_shell_settled_by_name",
			// Once every displayed shell (Background ones included) is
			// settled BY NAME, "starting" on screen can only come from a
			// durable source -- status_theme.feature's fixed shape.
			feature: `Feature: fixture
  Background:
    Given deck client "A" is started
    When deck client "A" creates shell session "fixed-anchor"
    Then within one configured reconcile interval deck client "A" row "fixed-anchor" contains "running"
    When deck client "A" creates shell session "fixed-target"
    And within one configured reconcile interval deck client "A" row "fixed-target" contains "running"
    And deck client "A" creates claude session "fixed-agent" with permission profile "safe"

  Scenario: generic starting wait with every shell settled
    When the state database session "fixed-agent" has status "starting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "starting"
    And deck client "A" captures its frame as "fixed-frame"
    Then deck client "A" frame still matches the captured "fixed-frame" frame
`,
		},
		{
			name: "store_only_transient_assertion_on_a_shell",
			// A store-only "starting" assertion on a created shell names
			// no screen at all, so it is never a frame waypoint.
			feature: `Feature: fixture
  Scenario: store-only starting assertion on a shell
    When deck client "A" creates shell session "store-transient"
    Then the state database contains session "store-transient" with status "starting"
    And within one configured reconcile interval the state database session "store-transient" is "starting" from "tmux" with killed_by_user=0
`,
		},
		{
			name: "single_shell_settled_by_name",
			// cure-01-01-2: the sole shell's own session-specific settle --
			// what replaces the generic wait the guard no longer trusts even
			// for one shell (preview.feature's and mouse.feature's
			// passive-preview wheel no-op scenarios use exactly this).
			feature: `Feature: fixture
  Scenario: the only shell created is settled by its own row
    Given deck client "A" is started
    When deck client "A" creates shell session "solo-shell"
    And within one configured reconcile interval deck client "A" row "solo-shell" contains "running"
    And deck client "A" captures its frame as "solo-frame"
    Then deck client "A" frame still matches the captured "solo-frame" frame
`,
		},
		{
			name: "shell_settled_by_name_beside_running_agent",
			// cure-01-01-2: a shell beside an already-running agent session
			// is fine once the shell's OWN row is waited on -- the guard
			// bans ambiguous generic settles, not mixing shells and agents.
			feature: `Feature: fixture
  Scenario: a new shell beside a running agent is settled by its own row
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "mixed-agent" with permission profile "safe"
    And within one configured reconcile interval deck client "A" screen contains "running"
    When deck client "A" creates shell session "mixed-shell"
    And within one configured reconcile interval deck client "A" row "mixed-shell" contains "running"
    And deck client "A" captures its frame as "mixed-frame"
    Then deck client "A" frame still matches the captured "mixed-frame" frame
`,
		},
		{
			name: "shell_settled_by_name_before_agent_session",
			// A shell settled by its own row stays settled when an agent
			// session is created afterward, and a later name-scoped settle
			// is unaffected by names that do not contain it.
			feature: `Feature: fixture
  Scenario: a shell settled by name before any agent session exists
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates shell session "first-shell"
    And within one configured reconcile interval deck client "A" row "first-shell" contains "running"
    And deck client "A" creates claude session "later-agent" with permission profile "safe"
    And deck client "A" captures its frame as "first-shell-frame"
    Then deck client "A" frame still matches the captured "first-shell-frame" frame
`,
		},
		{
			name: "grouped_named_wait_is_genuinely_durable_despite_the_group_name",
			// cure-01-01-3: a session-specific row wait naming the shell itself
			// is durable whatever a GROUP happens to be named, because the
			// runtime row helper (frameSidebarRowContains) now excludes group
			// header cells entirely (sidebarCellIsGroupHeader) -- a group
			// named "alpha-running" can no longer satisfy a wait on row
			// "alpha" contains "running" by itself, so this settle proves
			// alpha's own row exactly like any other named settle, and the
			// guard need not flag the coincidence as ambiguous any more (see
			// TestR150RowCallbackIgnoresGroupHeaderText for the runtime half).
			feature: `Feature: fixture
  Scenario: a group named after the shell's own settle text is not ambiguous
    Given deck client "A" is started
    When deck client "A" creates shell session "alpha"
    And the state database session "alpha" is in group "alpha-running"
    And within one configured reconcile interval deck client "A" row "alpha" contains "running"
    And deck client "A" captures its frame as "grouped-frame"
    Then deck client "A" frame still matches the captured "grouped-frame" frame
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

// TestR150RowStepSearchesTheSidebarCellOnly (cure-01-01-2) pins the runtime
// half of a session-specific settle: the row-scoped status steps
// (frameSidebarRowContains) must not be satisfied by preview-panel text
// that merely shares a terminal line with the named session's sidebar row.
func TestR150RowStepSearchesTheSidebarCellOnly(t *testing.T) {
	frame := strings.Join([]string{
		"+ deck - sessions -----------------+  alpha --------------------------------+",
		"| socket: deck_test_123_2          | $ echo running                        |",
		"| v default  (1)                   | running                               |",
		"| > alpha starting                 | running                               |",
		"|   1s ago                         | $                                     |",
		"+----------------------------------+---------------------------------------+",
	}, "\n")
	if frameSidebarRowContains(frame, "alpha", "running") {
		t.Fatalf("row \"alpha\" still shows \"starting\"; the preview's \"running\" on the same terminal line must not satisfy the row step:\n%s", frame)
	}
	if !frameSidebarRowContains(frame, "alpha", "starting") {
		t.Fatalf("row \"alpha\" shows \"starting\" in its own sidebar cell; the row step must match it:\n%s", frame)
	}
	settled := strings.Replace(frame, "> alpha starting ", "> alpha running  ", 1)
	if !frameSidebarRowContains(settled, "alpha", "running") {
		t.Fatalf("row \"alpha\" shows \"running\" in its own sidebar cell; the row step must match it:\n%s", settled)
	}
	unicode := strings.NewReplacer("|", "│").Replace(settled)
	if !frameSidebarRowContains(unicode, "alpha", "running") {
		t.Fatalf("the row step must read the sidebar cell of a Unicode-bordered frame too:\n%s", unicode)
	}
	if frameSidebarRowContains(strings.NewReplacer("|", "│").Replace(frame), "alpha", "running") {
		t.Fatalf("Unicode-bordered preview text must not satisfy the row step either")
	}
}
