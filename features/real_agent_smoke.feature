@real-agents
Feature: Real Claude agent session smoke test
  This scenario proves, against an actually installed Claude CLI (not the
  fake-claude fixture used everywhere else), that deck assigns a UUID
  conversation id at create time and passes that exact id back on resume.
  It deliberately runs only when a real `claude` binary is on PATH, so the
  default suite (which has none) is unaffected. Run it from the repository root
  with `DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...`
  (also documented in docs/reports/phase1.md). Upstream fields are asserted
  without aliases or coercion: an incompatible Claude upgrade is a visible
  conformance failure, not something the harness silently normalizes.

  Scenario: create a real claude session and resume it with the same conversation id
    Given the installed Claude CLI is available
    And deck client "A" is started
    When deck client "A" creates claude session "real claude one" with permission profile "safe"
    Then the state database session "real claude one" has a non-empty conversation id
    And the audit log's most recent launch argv for session "real claude one" contains "--session-id"
    When deck client "A" kills its selected session
    Then the state database contains session "real claude one" with status "stopped"
    When deck client "A" presses r on session "real claude one"
    Then the audit log's most recent launch argv for session "real claude one" contains "--resume"
    And the audit log's most recent launch argv for session "real claude one" contains session "real claude one"'s conversation id
    When deck client "A" exits cleanly

  Scenario: real claude accepts injected hooks and supplies the upstream payload contract
    Given the installed Claude CLI is available
    And deck client "A" is started
    When deck client "A" creates claude session "real claude hooks" with permission profile "safe"
    Then session "real claude hooks"'s launch instrumentation routes "SessionStart" to the released deck _hook
    And session "real claude hooks" receives a conforming real Claude "SessionStart" hook
    When deck client "A" exits cleanly
