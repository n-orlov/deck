@create-session
Feature: The create modal's text fields are edited with a real caret (requirement 178, SPEC §11.4 and §11.11)
  On a focused text field left and right move the caret and space types a
  space; the dialog's cycling keys apply to selection fields only. A key
  that used to be swallowed by the modal's Agent cycle therefore must not
  change the agent while the name field has focus.

  Scenario: left and right on the name field move the caret and leave the agent selection alone
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" opens the create modal
    Then deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "A" types "abc" into the create name field
    Then deck client "A" create name caret is after "abc"
    When deck client "A" presses left 2 times in the create name field
    Then deck client "A" create name caret is on "b"
    And deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "A" types "X" into the create name field
    Then deck client "A" screen contains "Name: aXbc"
    And deck client "A" create name caret is on "b"
    When deck client "A" presses right 3 times in the create name field
    Then deck client "A" create name caret is after "aXbc"
    And deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "A" types "Y" into the create name field
    Then deck client "A" screen contains "Name: aXbcY"
    And deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
