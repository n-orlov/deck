@status-attach
Feature: Attaching is inert on status and seen state
  Looking at a session never changes it (R320). Both ways to reach a
  session's pane, `a`'s full attach and Enter's interactive preview
  (SPEC 11.9), leave a waiting or error row exactly as it was: the same
  status, source and notify_epoch, still unseen, and no attached event.
  The unseen marker stays until the seen state is set by engagement.
  Covering test of the rewritten scenarios: TestAttachIsInertOnStatusSeenAndPosition
  (internal/tui/attach_inert_test.go).

  Scenario: attach leaves a waiting row waiting and unseen
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer prompt" with permission profile "safe"
    And the released running hook fires for session "answer prompt"
    And the released waiting hook fires for session "answer prompt"
    Then the state database session "answer prompt" is "waiting" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "answer prompt" is "waiting" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    When deck client "A" exits cleanly

  Scenario: entering the interactive preview leaves a waiting row waiting and unseen
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer inline" with permission profile "safe"
    And the released running hook fires for session "answer inline"
    And the released waiting hook fires for session "answer inline"
    Then the state database session "answer inline" is "waiting" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    When deck client "A" selects session "answer inline"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And the state database session "answer inline" is "waiting" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly

  Scenario: attach leaves a live error unseen with its verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "failed prompt" with permission profile "safe"
    And fake Claude session "failed prompt" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "failed prompt" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "failed prompt" has hook status "error", reason "tool_failure", message "", unseen, and notify_epoch 0
    And within one configured reconcile interval deck client "A" row "failed prompt" contains "!"
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "failed prompt" is "error" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    And after one configured reconcile interval deck client "A" row "failed prompt" still contains "!"
    When deck client "A" exits cleanly

  Scenario: entering the interactive preview leaves a live error unseen with its verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "failed inline" with permission profile "safe"
    And fake Claude session "failed inline" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "failed inline" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "failed inline" has hook status "error", reason "tool_failure", message "", unseen, and notify_epoch 0
    And within one configured reconcile interval deck client "A" row "failed inline" contains "!"
    When deck client "A" selects session "failed inline"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And the state database session "failed inline" is "error" from "hook" and is unseen, with notify_epoch=0, and 0 attached events
    When deck client "A" leaves interactive mode
    Then after one configured reconcile interval deck client "A" row "failed inline" still contains "!"
    When deck client "A" exits cleanly
