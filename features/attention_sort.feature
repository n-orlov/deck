@attention-sort
Feature: The attention sort, workspace grouping/collapse, and `space` (requirement 40)
  SPEC §11 requirements 28-32: sessions render in exactly one attention-driven
  order (waiting oldest-first, then error, running, starting, idle, stopped),
  grouped by workspace with collapsible headers, and `space` walks whatever
  needs attention from the one shared source the sort and the collapsed
  strip's count also use, without ever writing a session's status.

  Background:
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started

  @requirement-28-attention-order @requirement-29-attention-tie-break
  Scenario: sessions render in the full waiting/error/running/starting/idle/stopped order, ties broken oldest-first
    When deck client "A" creates shell session "s-idle"
    And deck client "A" creates shell session "s-error"
    And deck client "A" creates shell session "s-waiting-new"
    And deck client "A" creates shell session "s-waiting-old"
    And deck client "A" creates shell session "s-running"
    And deck client "A" creates claude session "s-agent" with permission profile "safe"
    And deck client "A" creates shell session "s-stopped"
    And shell session "s-stopped" exits with status zero
    # "error" cannot be posed by writing the state database directly while
    # s-error's tmux pane is still alive: SPEC section 7's self-heal
    # (internal/service.reconcile's repairTerminalRowWithLivePane) treats a
    # bare error row paired with a live pane as an invariant violation and
    # repairs a shell row straight back to "running" on the very next
    # reconcile tick (task 703, review finding 1's fallout), exactly as it
    # already does for a raced "stopped" write above. A genuine nonzero
    # pane exit is instead collected and killed by reconcile, which is what
    # makes the row's "error" the real tmux.pane_dead transition.
    And shell session "s-error" exits with status 1
    And the state database session "s-idle" has status "idle" 50 seconds ago
    And the state database session "s-waiting-new" has status "waiting" 10 seconds ago
    And the state database session "s-waiting-old" has status "waiting" 90 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "s-error" contains "error"
    And deck client "A" screen shows sessions in this order:
      | s-waiting-old  |
      | s-waiting-new  |
      | s-error        |
      | s-running      |
      | s-agent        |
      | s-idle         |
      | s-stopped      |
    When deck client "A" exits cleanly

  @requirement-30-workspace-grouping
  Scenario: the sidebar groups sessions by workspace, with a header per group, and a keyboard toggle can collapse one
    When deck client "A" creates shell session "grp-a-1"
    And deck client "A" creates shell session "grp-a-2"
    And deck client "A" creates shell session "grp-b-1"
    And the state database session "grp-b-1" has workspace "second-workspace"
    Then within one configured reconcile interval deck client "A" screen contains "second-workspace"
    And deck client "A" screen contains "grp-a-1"
    And deck client "A" screen contains "grp-a-2"
    And deck client "A" screen contains "grp-b-1"
    When deck client "A" selects session "grp-b-1"
    And deck client "A" sends "c"
    Then deck client "A" screen stops containing "grp-b-1"
    And deck client "A" screen contains "second-workspace"
    And deck client "A" screen contains "grp-a-1"
    And deck client "A" screen contains "grp-a-2"
    When deck client "A" exits cleanly

  @requirement-30-workspace-grouping
  Scenario: collapsing and expanding the sidebar's only workspace group round-trips via two `c` presses
    When deck client "A" creates shell session "solo-a"
    And deck client "A" creates shell session "solo-b"
    And deck client "A" selects session "solo-a"
    And deck client "A" sends "c"
    Then deck client "A" screen stops containing "solo-a"
    And deck client "A" screen stops containing "solo-b"
    When deck client "A" sends "c"
    Then deck client "A" screen contains "solo-a"
    And deck client "A" screen contains "solo-b"
    When deck client "A" exits cleanly

  @requirement-30-top-bottom
  Scenario: g and G jump to the first and last visible row, skipping a collapsed group's hidden rows
    When deck client "A" creates shell session "gg-a-1"
    And deck client "A" creates shell session "gg-a-2"
    And deck client "A" creates shell session "gg-b-1"
    And the state database session "gg-b-1" has workspace "gg-second-workspace"
    And the state database session "gg-a-1" has status "waiting" 20 seconds ago
    And the state database session "gg-a-2" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "gg-second-workspace"
    When deck client "A" selects session "gg-b-1"
    And deck client "A" sends "g"
    Then deck client "A" has session "gg-a-1" selected
    When deck client "A" sends "G"
    Then deck client "A" has session "gg-b-1" selected
    When deck client "A" selects session "gg-b-1"
    And deck client "A" sends "c"
    Then deck client "A" screen stops containing "gg-b-1"
    When deck client "A" selects session "gg-a-2"
    And deck client "A" sends "G"
    Then deck client "A" has session "gg-a-2" selected
    When deck client "A" exits cleanly

  @requirement-31-attention-count @requirement-15-collapsed-strip
  Scenario: the collapsed strip's attention count matches the sort's own notion of attention
    When deck client "A" creates shell session "cnt-wait"
    And deck client "A" creates shell session "cnt-err"
    And deck client "A" creates shell session "cnt-idle"
    And deck client "A" creates shell session "cnt-run"
    And the state database session "cnt-wait" has status "waiting" 5 seconds ago
    # "error" cannot be posed by a raw state-database write while cnt-err's
    # pane is alive -- see the order scenario above (task 703). A genuine
    # nonzero pane exit reaches a durable "error" instead.
    And shell session "cnt-err" exits with status 1
    And the state database session "cnt-idle" has status "idle" 5 seconds ago
    Then within one configured reconcile interval deck client "A" row "cnt-wait" contains "waiting"
    And within one configured reconcile interval deck client "A" row "cnt-err" contains "error"
    And within one configured reconcile interval deck client "A" row "cnt-idle" contains "idle"
    When deck client "A" sends "|"
    And deck client "A" sends "|"
    And deck client "A" sends "|"
    Then deck client "A" collapsed strip shows attention count 2
    When deck client "A" exits cleanly

  @requirement-31-space-walk @requirement-32-space-no-status-change
  Scenario: `space` walks only what needs attention, wraps, and changes no session's status
    When deck client "A" creates shell session "sp-1"
    And deck client "A" creates shell session "sp-2"
    And deck client "A" creates shell session "sp-3"
    And the state database session "sp-1" has status "waiting" 5 seconds ago
    # "error" cannot be posed by a raw state-database write while sp-2's pane
    # is alive -- see the order scenario above (task 703). A genuine nonzero
    # pane exit reaches a durable "error" instead, and the explicit wait
    # below (rather than relying on the timing of the "waiting" check, which
    # names a different session) makes sure sp-2 has actually reached it
    # before the walk starts.
    And shell session "sp-2" exits with status 1
    And the state database session "sp-3" has status "idle" 1 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "sp-2" contains "error"
    And deck client "A" selects session "sp-1"
    And the state database status rows are captured as "before-space"
    When deck client "A" sends " "
    Then deck client "A" has session "sp-2" selected
    When deck client "A" sends " "
    Then deck client "A" has session "sp-1" selected
    And the state database status rows still match "before-space"
    When deck client "A" exits cleanly
