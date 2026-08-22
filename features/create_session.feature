@create-session
Feature: The create modal's §11.7 cwd prefill (requirement 12)
  The create modal's working-directory field opens pre-filled rather than
  blank: with any §11.7 recent_cwds history it shows the most recently
  promoted entry, labelled on screen as the last used so the user can tell
  it is a default and not something they typed; with no history at all it
  falls back to the directory deck itself was started in. Either way, the
  first keystroke in the field replaces the whole prefill rather than
  appending to it -- the user never has to clear a default they did not
  ask for before typing their own path.

  @requirement-12-no-history-startup-cwd
  Scenario: with no recent_cwds history the cwd field pre-fills with the directory deck started in
    Given deck client "A" is started in a fresh directory labelled "start"
    When deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "start"
    And deck client "A" screen does not contain "(last used)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-12-last-used-prefill
  Scenario: the cwd field pre-fills with the most recent recent_cwds entry, labelled "last used"
    Given deck client "A" is started
    When deck client "A" creates shell session "cs-seed-recent" with a fresh working directory labelled "recent"
    And deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "recent"
    And deck client "A" screen contains "(last used)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-12-wholesale-replace
  Scenario: typing in the cwd field replaces the prefill wholesale rather than appending to it
    Given deck client "A" is started
    When deck client "A" creates shell session "cs-seed-recent2" with a fresh working directory labelled "recent2"
    And deck client "A" creates shell session "cs-typed-over" typing over the prefilled working directory with the directory labelled "typed"
    Then the state database session "cs-typed-over" has cwd exactly the directory labelled "typed"
    When deck client "A" exits cleanly
