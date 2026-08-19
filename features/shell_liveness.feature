@shell-liveness
Feature: Shell liveness does not fabricate agent status
  A live shell can be called running because its pane is its only signal, while
  a live coding-agent pane stays starting until an agent signal or probe arrives.

  Scenario: a live shell is promoted within one reconcile interval
    Given deck client "A" is started
    When deck client "A" creates shell session "live shell"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" screen does not contain "awaiting signal"
    And the state database contains session "live shell" with status "running"
    When deck client "A" exits cleanly

  Scenario: a live unsignalled agent remains starting
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "unsignalled" with permission profile "safe"
    Then after one configured reconcile interval deck client "A" screen still contains "starting - awaiting signal"
    And deck client "A" screen does not contain "running"
    And the state database contains session "unsignalled" with status "starting"
    And the private tmux session "deck_unsignalled" exists
    When deck client "A" exits cleanly
