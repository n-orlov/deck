@status-attach
Feature: Attaching records the selected attention episode
  Enter is an observed user action, not merely an acknowledgement. A waiting
  row is answered atomically, while an error row keeps its verdict; both clear
  their unseen marker before deck yields the terminal to tmux.

  Scenario: attach clears waiting and acknowledges it in one transition
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer prompt" with permission profile "safe"
    And the released waiting hook fires for session "answer prompt"
    Then the state database session "answer prompt" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "answer prompt" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When deck client "A" exits cleanly

  Scenario: attach acknowledges a live error without replacing its verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "failed prompt" with permission profile "safe"
    And fake Claude session "failed prompt" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "failed prompt" has hook status "error", reason "tool_failure", message "", acknowledged 0, and notify_epoch 0
    And within one configured reconcile interval deck client "A" row "failed prompt" contains "!"
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "failed prompt" is "error" from "hook" with acknowledged=1, notify_epoch=0, and 1 attached event
    And after one configured reconcile interval deck client "A" row "failed prompt" does not contain "!"
    When deck client "A" exits cleanly
