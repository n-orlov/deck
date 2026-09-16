@codex
Feature: Codex hook-driven identity and status
  Codex mints its own conversation id on its first prompt, never at launch
  (SPEC §8.2): a freshly created row carries none yet, is classified purely
  from its sampled pane text, and refuses resume/restart rather than
  guessing one. Once its own SessionStart hook reports an id, deck adopts
  it and the row's status becomes hook-driven ("live"); a later
  PermissionRequest hook lands the row in "waiting" with the requesting
  tool's own name as its reason (SPEC §13.4's @codex scenario). Every step
  here runs against cmd/fake-codex on the fixture PATH -- no real codex-cli
  binary is present or required.

  Scenario: two Codex rows in one directory adopt their own ids and report a real approval
    Given the deck config probes agent panes quickly
    And a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates codex session "one" with permission profile "safe"
    And deck client "A" creates codex session "two" with permission profile "safe"
    Then the state database session "one" has no conversation id
    And the state database session "two" has no conversation id
    And within 3 seconds deck client "A" row "one" contains "sampled"
    And within 3 seconds deck client "A" row "two" contains "sampled"
    When deck client "A" presses R on session "one"
    Then deck client "A" screen contains "Cannot restart: codex has not started a conversation yet"
    When deck client "A" presses R on session "two"
    Then deck client "A" screen contains "Cannot restart: codex has not started a conversation yet"

    When fake Codex session "one" is prompted with "hello from one"
    And fake Codex session "two" is prompted with "hello from two"
    Then within 3 seconds deck client "A" row "one" contains "live"
    And within 3 seconds deck client "A" row "two" contains "live"
    And the state database session "one" has a non-empty conversation id
    And the state database session "two" has a non-empty conversation id
    And the state database sessions "one" and "two" have different conversation ids
    And session "one"'s codex transcript does not mention session "two"'s conversation id
    And session "two"'s codex transcript does not mention session "one"'s conversation id

    When fake Codex session "one" requests approval to run tool "apply_patch"
    Then within 3 seconds deck client "A" row "one" contains "waiting"
    And the state database session "one"'s status reason contains "apply_patch"
    And deck client "A" row "two" does not contain "waiting"
    When deck client "A" exits cleanly
