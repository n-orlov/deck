Feature: Distinct conversation ids in one working directory
  SPEC §13.4's identity guarantee does not depend on separate directories:
  two agent sessions sharing one cwd must still be assigned distinct
  conversation ids, and resuming one must never leak the other's id into
  its argv.

  Scenario: two claude sessions in one cwd keep separate conversation ids
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "one" with permission profile "safe" and message "one's message"
    And deck client "A" creates claude session "two" with permission profile "safe" and message "two's message"
    Then the state database contains session "one" with status "stopped"
    And the state database contains session "two" with status "stopped"
    And the state database session "one" has a non-empty conversation id
    And the state database session "two" has a non-empty conversation id
    And the state database sessions "one" and "two" have different conversation ids
    When deck client "A" presses r on session "one"
    Then deck client "A" screen contains "starting"
    And the audit log's most recent launch argv for session "one" contains "--resume"
    And the audit log's most recent launch argv for session "one" does not contain session "two"'s conversation id
    And session "one" replays its own last message, not session "two"'s
    When deck client "A" exits cleanly
