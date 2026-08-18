@multiclient
Feature: Launch-lease race
  Multiple independent deck clients racing the resume key on the same
  stopped row must converge on exactly one real launch (SPEC §9.3).

  Scenario: three clients racing resume on one row produce exactly one launch
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates shell session "race target"
    And deck client "A" kills its selected session
    Then the state database contains session "race target" with status "stopped"
    When deck clients "A", "B" and "C" race pressing r on session "race target"
    Then at least one of deck clients "A", "B" and "C" screen contains "starting elsewhere"
    And within one configured reconcile interval deck client "A" screen contains "starting - awaiting signal"
    And within one configured reconcile interval deck client "B" screen contains "starting - awaiting signal"
    And within one configured reconcile interval deck client "C" screen contains "starting - awaiting signal"
    And exactly 1 private tmux session matches slug "deck_race-target"
    And the audit log has 2 launch records for session "race target"
    And deck client "A" screen does not contain "running"
    And deck client "B" screen does not contain "running"
    And deck client "C" screen does not contain "running"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly
