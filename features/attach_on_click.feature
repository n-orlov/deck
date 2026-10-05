@attach-on-click
Feature: §11.8 [ui] attach_on_click -- a sidebar click only selects when it is off (GH #62)
  With [ui] attach_on_click off, a click and a double-click on a sidebar row
  select it and nothing else: the list keeps keyboard focus, the preview
  stays passive, and the next key goes to the list.

  @gh-62-attach-on-click-off-click
  Scenario: with attach_on_click off, a click on a row selects it and the next j moves the selection without reaching the pane
    Given every deck client in this scenario only selects on a sidebar click
    And deck client "A" is started
    When deck client "A" creates shell session "aoc-alpha"
    And deck client "A" creates shell session "aoc-bravo"
    And within one configured reconcile interval deck client "A" row "aoc-alpha" contains "running"
    And within one configured reconcile interval deck client "A" row "aoc-bravo" contains "running"
    And deck client "A" clicks on the row containing "aoc-alpha"
    Then deck client "A" has session "aoc-alpha" selected
    And deck client "A" screen does not contain "Ctrl+Q to leave"
    When deck client "A" sends "j"
    Then deck client "A" has session "aoc-bravo" selected
    And deck client "A" screen does not contain "Ctrl+Q to leave"
    And the private tmux pane for session "aoc-alpha" did not receive "j"
    And the private tmux pane for session "aoc-bravo" did not receive "j"
    When deck client "A" exits cleanly

  @gh-62-attach-on-click-off-double-click
  Scenario: with attach_on_click off, a double-click on a row selects it and the next j moves the selection without reaching the pane
    Given every deck client in this scenario only selects on a sidebar click
    And deck client "A" is started
    When deck client "A" creates shell session "aod-alpha"
    And deck client "A" creates shell session "aod-bravo"
    And within one configured reconcile interval deck client "A" row "aod-alpha" contains "running"
    And within one configured reconcile interval deck client "A" row "aod-bravo" contains "running"
    And deck client "A" double-clicks on the row containing "aod-alpha"
    Then deck client "A" has session "aod-alpha" selected
    And deck client "A" screen does not contain "Ctrl+Q to leave"
    When deck client "A" sends "j"
    Then deck client "A" has session "aod-bravo" selected
    And deck client "A" screen does not contain "Ctrl+Q to leave"
    And the private tmux pane for session "aod-alpha" did not receive "j"
    And the private tmux pane for session "aod-bravo" did not receive "j"
    When deck client "A" exits cleanly
