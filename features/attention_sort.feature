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
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). s-error's pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair, which already handled the "s-stopped" row above) is what
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
    And the state database session "grp-b-1" is in group "second-workspace"
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
    And the state database session "gg-b-1" is in group "gg-second-workspace"
    And the state database session "gg-a-1" has status "waiting" 20 seconds ago
    And the state database session "gg-a-2" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "gg-second-workspace"
    # R129 (task 011) orders the sidebar's groups alphabetically with the
    # implicit default group ALWAYS last, so the render is
    # "gg-second-workspace" (gg-b-1) first, then "default" (gg-a-1 then
    # gg-a-2, attention order within the group). g/G therefore land on
    # gg-b-1 and gg-a-2 -- the pre-R129 expectations here (gg-a-1 first,
    # gg-b-1 last) belonged to the group order this task replaced.
    When deck client "A" selects session "gg-a-2"
    And deck client "A" sends "g"
    # R137/D.1 (task 012) made a group header a cursor stop of its own (SPEC
    # §11: "`↑`/`↓` and the rest of §11.3's list navigation land on headers as
    # well as on session rows"), so `g` now lands on the FIRST stop -- the
    # "gg-second-workspace" header -- rather than on the first row under it.
    # One `↓` from there is gg-b-1, which is what pins "g went to the very
    # top": before R137 the same two keys would have left the cursor on
    # gg-a-1, the second row. The header cursor itself is not asserted
    # directly because the sidebar renders no selection marker on a header.
    And deck client "A" sends "j"
    Then deck client "A" has session "gg-b-1" selected
    When deck client "A" sends "G"
    # The last stop is a ROW here (nothing is collapsed yet): the default
    # group renders last and gg-a-2 is its bottom row, so G's own landing
    # place is unchanged by headers becoming stops.
    Then deck client "A" has session "gg-a-2" selected
    When deck client "A" selects session "gg-a-1"
    And deck client "A" sends "c"
    Then deck client "A" screen stops containing "gg-a-2"
    When deck client "A" selects session "gg-b-1"
    And deck client "A" sends "G"
    # With `default` folded its rows are hidden, so the last stop is that
    # folded group's own header -- again a stop that did not exist before
    # R137. One `↑` from it is gg-b-1, the last VISIBLE row, which is what
    # this step always meant to pin.
    And deck client "A" sends "k"
    Then deck client "A" has session "gg-b-1" selected
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
