@create-session
Feature: The focused text field draws a reverse-video caret cell that moves with the left key (requirement 181, SPEC §11.11)
  The caret is one cell in reverse video, SGR 7, on the focused field only. It
  is read here straight off a real client's grid with the per-cell attribute
  step, so a stand-in such as a trailing "_" or a hardware cursor alone would
  not satisfy it.

  Scenario: the caret cell sits after the typed text and moves one cell left with the left key
    Given deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "abc" into the create name field
    Then deck client "A" create name caret is after "abc"
    And deck client "A" cell at row 2 column 13 is reverse
    And deck client "A" cell at row 2 column 12 is not reverse
    When deck client "A" presses left 1 times in the create name field
    Then deck client "A" create name caret is on "c"
    And deck client "A" cell at row 2 column 12 is reverse
    And deck client "A" cell at row 2 column 13 is not reverse
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
