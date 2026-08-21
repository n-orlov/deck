Feature: §11.2 layout modes, their keys, and their persistence
  auto chooses side-by-side at or above deck's own 80-column minimum and
  stacked below it; `|` cycles auto → side-by-side → stacked → collapsed →
  auto; `<`/`>` step sidebar_width by one column, clamped; a pinned mode
  that cannot hold its floors falls back to auto for that frame only, and
  returns once the terminal can hold it again; layout_mode and
  sidebar_width persist in state.db (never config.toml) across a restart.

  @requirement-38-layout-modes
  Scenario Outline: auto selects side-by-side at and above 80 columns, stacked below it
    Given deck client "solo" is started with terminal size <width>x30
    Then deck client "solo" layout is "<layout>"
    And deck client "solo" exits cleanly

    Examples:
      | width | layout       |
      | 79    | stacked      |
      | 80    | side-by-side |
      | 81    | side-by-side |

  @requirement-38-layout-modes
  Scenario: | cycles auto -> side-by-side -> stacked -> collapsed -> auto
    Given deck client "solo" is started with terminal size 100x30
    Then deck client "solo" layout is "side-by-side"
    When deck client "solo" sends "|"
    Then deck client "solo" layout is "side-by-side"
    When deck client "solo" sends "|"
    Then deck client "solo" layout is "stacked"
    When deck client "solo" sends "|"
    Then deck client "solo" layout is "collapsed"
    When deck client "solo" sends "|"
    Then deck client "solo" layout is "side-by-side"
    And deck client "solo" exits cleanly

  @requirement-38-layout-modes
  Scenario: a mid-scenario resize re-chooses auto's mode
    Given deck client "solo" is started with terminal size 100x30
    Then deck client "solo" layout is "side-by-side"
    When deck client "solo" terminal is resized to 60x20
    Then deck client "solo" layout is "stacked"
    When deck client "solo" terminal is resized to 100x30
    Then deck client "solo" layout is "side-by-side"
    And deck client "solo" exits cleanly

  @requirement-38-layout-modes
  Scenario: a pinned side-by-side mode falls back below its floors and returns when the terminal does
    Given deck client "solo" is started with terminal size 100x30
    When deck client "solo" sends "|"
    Then deck client "solo" layout is "side-by-side"
    When deck client "solo" terminal is resized to 50x20
    Then deck client "solo" layout is "stacked"
    When deck client "solo" terminal is resized to 100x30
    Then deck client "solo" layout is "side-by-side"
    And deck client "solo" exits cleanly

  @requirement-38-layout-modes
  Scenario: < and > clamp sidebar_width at both ends
    Given deck client "solo" is started with terminal size 100x30
    When deck client "solo" presses "<" 20 times
    Then deck client "solo" sidebar seam column is captured as "floor"
    When deck client "solo" presses "<" 5 times
    Then deck client "solo" sidebar seam column still matches the captured "floor"
    When deck client "solo" presses ">" 100 times
    Then deck client "solo" sidebar seam column is captured as "ceiling"
    When deck client "solo" presses ">" 5 times
    Then deck client "solo" sidebar seam column still matches the captured "ceiling"
    And deck client "solo" exits cleanly

  @requirement-14
  Scenario: the below-minimum notice lands on the footer line and only there
    Given deck client "solo" is started with terminal size 100x30
    Then deck client "solo" screen does not contain "below deck's supported minimum"
    When deck client "solo" terminal is resized to 70x24
    Then deck client "solo" screen contains "below deck's supported minimum"
    And deck client "solo" screen contains "deck - sessions"
    When deck client "solo" terminal is resized to 80x24
    Then deck client "solo" layout is "side-by-side"
    And deck client "solo" screen does not contain "below deck's supported minimum"
    And deck client "solo" exits cleanly

  @requirement-38-layout-modes
  Scenario: layout_mode and sidebar_width persist across a restart, with config.toml unchanged
    Given the deck config allows yolo
    And the scenario's config.toml is captured as "before"
    And deck client "solo" is started with terminal size 100x30
    And deck client "solo" presses ">" 3 times
    And deck client "solo" sidebar seam column is captured as "widened"
    And deck client "solo" sends "|"
    And deck client "solo" sends "|"
    Then deck client "solo" layout is "stacked"
    And the scenario's config.toml still matches the captured "before"
    And deck client "solo" exits cleanly
    When deck client "solo" is restarted with terminal size 100x30
    Then deck client "solo" layout is "stacked"
    When deck client "solo" sends "|"
    And deck client "solo" sends "|"
    And deck client "solo" sends "|"
    Then deck client "solo" layout is "side-by-side"
    And deck client "solo" sidebar seam column still matches the captured "widened"
    And the scenario's config.toml still matches the captured "before"
    And deck client "solo" exits cleanly
