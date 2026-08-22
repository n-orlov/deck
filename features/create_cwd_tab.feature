@create-session
Feature: The create modal's cwd field follows bash's tab-completion contract (requirement 16)
  Tab completes to the longest common prefix shared by every directory
  candidate for the segment being completed, when that advances the text
  already typed; when it cannot advance any further and more than one
  candidate remains, tab instead lists the candidates for selection, and
  choosing one puts it in the field.

  @requirement-16-tab-advances-common-prefix
  Scenario: tab completes to the longest common prefix when that advances the text
    Given a scratch directory labelled "tabprefix" exists
    And a directory named "prefixaaa" exists in the scratch directory labelled "tabprefix"
    And a directory named "prefixbbb" exists in the scratch directory labelled "tabprefix"
    And deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "tabprefix" followed by "pre" into the cwd field
    And deck client "A" presses "tab" in the cwd field
    Then deck client "A" screen contains the scratch directory labelled "tabprefix" plus "/prefix"
    And deck client "A" screen does not contain "prefixaaa"
    And deck client "A" screen does not contain "prefixbbb"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-16-tab-lists-candidates-and-selects
  Scenario: tab lists the candidates when it cannot advance the text further, and selecting one fills the field
    Given a scratch directory labelled "tablist" exists
    And a directory named "prefixaaa" exists in the scratch directory labelled "tablist"
    And a directory named "prefixbbb" exists in the scratch directory labelled "tablist"
    And deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "tab-list-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "tablist" followed by "prefix" into the cwd field
    And deck client "A" presses "tab" in the cwd field
    Then deck client "A" screen contains "prefixaaa/"
    And deck client "A" screen contains "prefixbbb/"
    When deck client "A" presses "down" in the cwd field
    And deck client "A" presses "enter" in the cwd field
    Then deck client "A" screen contains the scratch directory labelled "tablist" plus "/prefixbbb/"
    When deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "tab-list-session" has cwd exactly the scratch directory labelled "tablist" plus "/prefixbbb/"
    When deck client "A" exits cleanly

  @requirement-16-tab-does-nothing-when-already-unique
  Scenario: tab falls through to the next field when the segment already names the one candidate in full
    Given a scratch directory labelled "tabunique" exists
    And a directory named "onlyoneprefix" exists in the scratch directory labelled "tabunique"
    And deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "tabunique" followed by "onlyoneprefix" into the cwd field
    And deck client "A" presses "tab" in the cwd field
    Then deck client "A" screen contains "> Agent:"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
