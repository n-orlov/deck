@multiclient
Feature: Forced entry into interactive mode answers a waiting row
  `F` (SPEC "F forces entry over whoever holds the window") is identical to
  plain Enter in every respect but two: it does not refuse for another
  client's claim and it steals ownership over a live holder instead of
  standing down for one (task 105). Stealing the window is still a
  deck-mediated attachment in exactly the sense SPEC section 7 gives `a`
  and plain Enter: it answers a waiting row and records an attached event
  at the client that forced its way in, using the same durable transaction
  (task 109), not a degraded or partial one.

  Scenario: a forced entry over a waiting row answers it at the forcing client
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    And deck client "B" is started
    When deck client "A" creates claude session "contested entry" with permission profile "safe"
    Then within one configured reconcile interval deck client "B" screen contains "contested entry"
    When the released running hook fires for session "contested entry"
    And the released waiting hook fires for session "contested entry"
    Then the state database session "contested entry" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    When deck client "A" selects session "contested entry"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When within one configured reconcile interval deck client "B" row "contested entry" contains "running"
    And deck client "B" selects session "contested entry"
    And deck client "B" forces entry into interactive mode
    Then deck client "B" screen contains "Ctrl+Q"
    And the state database session "contested entry" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When deck client "B" leaves interactive mode
    And deck client "A" leaves interactive mode
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly
