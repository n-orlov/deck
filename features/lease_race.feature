@multiclient
Feature: Launch-lease race
  Multiple independent deck clients racing the resume key on the same
  stopped row must converge on exactly one real launch (SPEC §9.3).

  Scenario: three clients racing resume on one row produce exactly one launch
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates claude session "race target" with permission profile "safe"
    And deck client "A" kills its selected session
    Then the state database contains session "race target" with status "stopped"
    When deck client "A" creates claude session "unsignalled agent" with permission profile "safe"
    Then the state database contains session "unsignalled agent" with status "starting"
    When deck clients "A", "B" and "C" race pressing r on session "race target"
    Then at least one of deck clients "A", "B" and "C" screen contains "starting elsewhere"
    And within one configured reconcile interval deck client "A" row "race target" contains "starting"
    And within one configured reconcile interval deck client "B" row "race target" contains "starting"
    And within one configured reconcile interval deck client "C" row "race target" contains "starting"
    And exactly 1 private tmux session matches slug "deck_race-target"
    And the audit log has 2 launch records for session "race target"
    When fake Claude session "race target" fires "SessionStart" for itself using injected identity:
      | source | resume |
    Then within one configured reconcile interval deck client "A" row "race target" contains "running"
    And within one configured reconcile interval deck client "B" row "race target" contains "running"
    And within one configured reconcile interval deck client "C" row "race target" contains "running"
    # This separate live agent has sent no hook or probe signal, so none of the
    # clients may fabricate running merely because its pane exists.
    And deck client "A" row "unsignalled agent" does not contain "running"
    And deck client "B" row "unsignalled agent" does not contain "running"
    And deck client "C" row "unsignalled agent" does not contain "running"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly
