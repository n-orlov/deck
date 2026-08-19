@status-attach
Feature: Attaching answers the selected waiting episode
  Enter is an observed user action, not merely an acknowledgement. When deck
  attaches to a waiting row it atomically clears that episode before yielding
  the terminal to tmux.

  Scenario: attach clears waiting and acknowledges it in one transition
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "answer prompt" with permission profile "safe"
    And the released waiting hook fires for session "answer prompt"
    Then the state database session "answer prompt" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "answer prompt" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When deck client "A" exits cleanly
