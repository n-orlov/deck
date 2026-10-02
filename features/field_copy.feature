@create-session
Feature: alt+w copies a focused field's whole text (requirement 180, SPEC §11.8 and §11.11)
  alt+w on a focused text field copies the field's whole text through the same
  path drag-to-copy uses -- deck's own tmux buffer, "deck-selection" -- and
  shows drag-to-copy's own confirmation on screen. The buffer lives on deck's
  own tmux server, so the scenario first creates a session to have one running.

  Scenario: alt+w on the create modal's name field puts its text in deck's own selection buffer
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates shell session "copy-host"
    Then deck client "A" screen contains "copy-host"
    When deck client "A" opens the create modal
    And deck client "A" types "copy-me" into the create name field
    Then deck client "A" screen contains "Name: copy-me"
    When deck client "A" presses left 3 times in the create name field
    And deck client "A" presses alt+w in the create name field
    Then deck client "A" screen contains "Copied 7 bytes to deck's own tmux buffer"
    And deck's own selection buffer holds exactly "copy-me"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
