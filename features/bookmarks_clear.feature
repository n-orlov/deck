@bookmarks-clear
Feature: Clearing a bookmark (R288)
  With the bookmarks setting on, Alt+M then Delete removes the bookmark a
  session holds, and archiving a bookmarked session clears its slot too, so
  no slot keeps pointing at a row the list hides.

  Scenario: Alt+M then Delete clears the bookmark of the selected session
    Given every deck client in this scenario has the bookmarks setting on
    And deck client "A" is started
    When deck client "A" creates shell session "bmc-clear"
    And deck client "A" bookmarks its selected session in slot 3
    Then the state database holds bookmark slot 3 for session "bmc-clear"
    And deck client "A" screen contains "[3] bmc-clear"
    When deck client "A" presses Alt+M then Delete on its selected session, which holds slot 3
    Then the state database holds no bookmark slot 3
    And deck client "A" screen does not contain "[3]"
    When deck client "A" exits cleanly

  Scenario: archiving a bookmarked session clears its slot
    Given every deck client in this scenario has the bookmarks setting on
    And deck client "A" is started
    When deck client "A" creates shell session "bmc-archive"
    And deck client "A" bookmarks its selected session in slot 5
    Then the state database holds bookmark slot 5 for session "bmc-archive"
    When deck client "A" archives its selected session "bmc-archive"
    Then deck client "A" screen does not contain "bmc-archive"
    And the state database holds no bookmark slot 5
    When deck client "A" exits cleanly
