@real-agents
Feature: Real Claude agent session smoke test
  This scenario proves, against an actually installed Claude CLI (not the
  fake-claude fixture used everywhere else), that deck assigns a UUID
  conversation id at create time and passes that exact id back on resume.
  It deliberately runs only when a real `claude` binary is on PATH, so the
  default suite (which has none) is unaffected; see
  docs/reports/phase1.md for the one command that runs it.

  Scenario: create a real claude session and resume it with the same conversation id
    Given the installed Claude CLI is available
    And deck client "A" is started
    When deck client "A" creates claude session "real claude one" with permission profile "safe"
    Then the state database session "real claude one" has a non-empty conversation id
    And the audit log's most recent launch argv for session "real claude one" contains "--session-id"
    When deck client "A" presses r on session "real claude one"
    Then the audit log's most recent launch argv for session "real claude one" contains "--resume"
    And the audit log's most recent launch argv for session "real claude one" contains session "real claude one"'s conversation id
    When deck client "A" exits cleanly
