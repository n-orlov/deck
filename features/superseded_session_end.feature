@requirement-200-superseded-session-end
Feature: the replaced pane's session_end is not presented as a fault in the detail pane (R200, #64)
  A restart replaces the pane, and the old pane's own SessionEnd hook arrives
  late under the old launch generation. deck declines it (the
  session_end.superseded event, which E lists raw), which is correct and
  routine, so the i detail dialog shows no "Hook declined" line for it. A late
  Stop from the same replaced pane is not routine and stays loud.

  Scenario: restart then the old pane's SessionEnd leaves i without an alarm, while a late Stop still shows one
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started with terminal size 100x40
    When deck client "A" creates claude session "teardown quiet" with permission profile "safe"
    # A freshly created row holds no launch generation (create takes no lease),
    # so the first restart gives the row one; the second restart replaces the
    # pane that generation belongs to.
    And deck client "A" presses R on session "teardown quiet"
    Then within one configured reconcile interval the audit log has 2 launch records for session "teardown quiet"
    When the launch generation of session "teardown quiet" is remembered
    And deck client "A" presses R on session "teardown quiet"
    Then within one configured reconcile interval the audit log has 3 launch records for session "teardown quiet"
    When the released deck _hook receives "SessionEnd" for session "teardown quiet" carrying the remembered launch generation
    And deck client "A" opens the event log
    Then deck client "A" screen contains "session_end.superseded"
    When deck client "A" closes the dialog with escape
    And deck client "A" opens detail for session "teardown quiet"
    Then deck client "A" screen shows no "Hook declined" once the detail's declined-hook lookup has settled
    When deck client "A" closes detail
    And the released deck _hook receives "Stop" for session "teardown quiet" carrying the remembered launch generation
    And deck client "A" opens detail for session "teardown quiet"
    Then deck client "A" screen contains "Hook declined:"
    And deck client "A" screen contains "stop.superseded"
    When deck client "A" closes detail
    And deck client "A" exits cleanly
