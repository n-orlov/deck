@user-kill
Feature: User kill is terminal against later automation
  An explicit x kill is durable user intent. A delayed agent hook may remain
  useful evidence, but it must not resurrect the row behind the user's back.

  Scenario: a running hook cannot undo an explicit user kill
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "terminal kill" with permission profile "safe"
    Then the private tmux session "deck_terminal-kill" exists
    When deck client "A" kills its selected session
    Then the state database session "terminal kill" is "stopped" from "user" with killed_by_user=1
    When the released running hook fires for session "terminal kill"
    Then the state database session "terminal kill" is "stopped" from "user" with killed_by_user=1
    And the state database contains session "terminal kill" with status "stopped"
    When deck client "A" exits cleanly
