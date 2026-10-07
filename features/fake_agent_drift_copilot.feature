Feature: Fake Copilot drift alarm
  The fake Copilot's pane strings and hook payloads must keep matching the
  real-capture fixtures in internal/agent/testdata/probes/copilot/ and the
  payload shapes the #72 spike recorded for Copilot CLI 1.0.93. These scenarios
  need no real Copilot, so they run in normal CI.

  Scenario: fake Copilot screens carry the key strings of the recorded fixtures
    Given the fake Copilot fixture is built
    Then the fake Copilot "idle" screen carries the key strings of the "idle" fixture
    And the fake Copilot "working" screen carries the key strings of the "working" fixture
    And the fake Copilot "permission" screen carries the key strings of the "permission" fixture
    And the fake Copilot "question" screen carries the key strings of the "question" fixture
    And the fake Copilot "trust" screen carries the key strings of the "trust" fixture
    And the fake Copilot "error" screen carries the key strings of the "error" fixture

  Scenario: fake Copilot hook payloads carry the keys the #72 spike recorded
    Given the fake Copilot fixture is built
    When the fake Copilot plays a turn, a notification, an error and a clean exit
    Then the fake Copilot "userPromptSubmitted" payload carries the #72 keys
    And the fake Copilot "sessionStart" payload carries the #72 keys
    And the fake Copilot "notification" payload carries the #72 keys
    And the fake Copilot "agentStop" payload carries the #72 keys
    And the fake Copilot "errorOccurred" payload carries the #72 keys
    And the fake Copilot "sessionEnd" payload carries the #72 keys
