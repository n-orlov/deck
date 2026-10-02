@create-session
Feature: The create modal's cwd field is edited under a caret (requirement 179, SPEC §11.7)
  The cwd field opens on an offered value, so a caret or editing key accepts
  it and edits it in place: an offered path edited in the middle is the path
  the session is created in. The ghost completion and tab completion exist
  only with the caret at the END of the field: with the caret anywhere else
  no ghost is painted and tab changes nothing, and once the caret is back at
  the end the ghost returns and right accepts it.

  @requirement-179-offered-cwd-edited-mid-path
  Scenario: an offered cwd edited in the middle of its path is the cwd the session is created with
    Given deck client "A" is started in a fresh directory labelled "ep"
    And a directory named "create-session-xp" exists under the scenario home labelled "xp"
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-caret-edit-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" presses "left" in the cwd field
    And deck client "A" presses "backspace" in the cwd field
    And deck client "A" sends "x"
    And deck client "A" submits the create modal
    Then deck client "A" has session "cwd-caret-edit-session" selected
    And the state database session "cwd-caret-edit-session" has cwd exactly the directory labelled "xp"
    When deck client "A" exits cleanly

  @requirement-179-mid-field-no-ghost-tab-nothing
  Scenario: with the caret mid-field no ghost is painted and tab changes nothing
    Given a scratch directory labelled "mid" exists
    And a directory named "uniqueprojmid" exists in the scratch directory labelled "mid"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "mid" followed by "uniquep" into the cwd field
    Then deck client "A" text "rojmid/" has foreground token "hint"
    When deck client "A" presses "left" in the cwd field
    Then deck client "A" cwd field comes to show no ghost text
    When deck client "A" presses "tab" in the cwd field
    And deck client "A" presses "right" in the cwd field
    Then deck client "A" text "rojmid/" has foreground token "hint"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-179-back-at-end-ghost-returns-right-accepts
  Scenario: back at the end of the field the ghost returns and right accepts it
    Given a scratch directory labelled "back" exists
    And a directory named "uniqueprojback" exists in the scratch directory labelled "back"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-caret-back-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "back" followed by "uniquep" into the cwd field
    And deck client "A" presses "left" in the cwd field
    Then deck client "A" cwd field comes to show no ghost text
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" text "rojback/" has foreground token "hint"
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" screen contains "uniqueprojback/"
    When deck client "A" submits the create modal
    Then deck client "A" has session "cwd-caret-back-session" selected
    And the state database session "cwd-caret-back-session" has cwd exactly the scratch directory labelled "back" plus "/uniqueprojback/"
    When deck client "A" exits cleanly
