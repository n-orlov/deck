@status-attach
Feature: Attaching records the selected attention episode
  Attaching is an observed user action, not merely an acknowledgement, and
  both ways to reach a session's pane count: `a`'s full attach and Enter's
  interactive preview (SPEC 11.9). A waiting row is answered atomically,
  while an error row keeps its verdict; both clear their unseen marker the
  moment the keyboard reaches the pane.

  Scenario: attach clears waiting and acknowledges it in one transition
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer prompt" with permission profile "safe"
    And the released running hook fires for session "answer prompt"
    And the released waiting hook fires for session "answer prompt"
    Then the state database session "answer prompt" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "answer prompt" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When deck client "A" exits cleanly

  Scenario: entering the interactive preview clears waiting and acknowledges it like an attach
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer inline" with permission profile "safe"
    And the released running hook fires for session "answer inline"
    And the released waiting hook fires for session "answer inline"
    Then the state database session "answer inline" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    When deck client "A" selects session "answer inline"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And the state database session "answer inline" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly

  Scenario: attach acknowledges a live error without replacing its verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "failed prompt" with permission profile "safe"
    And fake Claude session "failed prompt" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "failed prompt" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "failed prompt" has hook status "error", reason "tool_failure", message "", acknowledged 0, and notify_epoch 0
    And within one configured reconcile interval deck client "A" row "failed prompt" contains "!"
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "failed prompt" is "error" from "hook" with acknowledged=1, notify_epoch=0, and 1 attached event
    And after one configured reconcile interval deck client "A" row "failed prompt" does not contain "!"
    When deck client "A" exits cleanly

  Scenario: entering the interactive preview acknowledges a live error without replacing its verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "failed inline" with permission profile "safe"
    And fake Claude session "failed inline" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "failed inline" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "failed inline" has hook status "error", reason "tool_failure", message "", acknowledged 0, and notify_epoch 0
    And within one configured reconcile interval deck client "A" row "failed inline" contains "!"
    When deck client "A" selects session "failed inline"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And the state database session "failed inline" is "error" from "hook" with acknowledged=1, notify_epoch=0, and 1 attached event
    When deck client "A" leaves interactive mode
    Then after one configured reconcile interval deck client "A" row "failed inline" does not contain "!"
    When deck client "A" exits cleanly
