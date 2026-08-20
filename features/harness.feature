Feature: Godog harness wiring
  The repository's normal Go test command discovers default feature files.

  Scenario: default feature suite is wired
    Given the Godog harness is available

  Scenario: a private tmux server can be removed
    Given the private tmux server is killed

  @requirement-6-eaw-placement
  Scenario: the screen emulator places an East-Asian-Wide cell without splitting it
    Given a fresh terminal emulator sized 12x4
    When the emulator receives "界┌"
    Then the emulator cell at column 0 has width 2 and content "界"
    And the emulator cell at column 1 is a continuation cell
    And the emulator cell at column 2 has content "┌"

  @requirement-1-resize
  Scenario: a mid-scenario terminal resize changes the grid the frame is read from
    Given deck client "solo" is started with terminal size 100x30
    And deck client "solo" frame width is 100
    And deck client "solo" frame height is 30
    When deck client "solo" terminal is resized to 60x20
    Then deck client "solo" frame width is 60
    And deck client "solo" frame height is 20
    And deck client "solo" exits cleanly

  @requirement-2-mouse-synthesis
  Scenario: SGR mouse reports are synthesized for click, double-click, wheel and drag
    Given deck client "solo" is started
    And deck client "solo" captures its frame as "before-mouse"
    When deck client "solo" clicks at column 5 row 3
    And deck client "solo" double-clicks at column 5 row 3
    And deck client "solo" scrolls the wheel up at column 5 row 3
    And deck client "solo" scrolls the wheel down at column 5 row 3
    And deck client "solo" drags from column 5 row 3 to column 50 row 10
    And deck client "solo" clicks at column 224 row 3
    Then deck client "solo" frame still matches the captured "before-mouse" frame
    And deck client "solo" exits cleanly

  @requirement-3-deck-mouse
  Scenario: DECK_MOUSE overrides the default mouse-reporting setting
    Given deck client "default-mouse" is started
    Then deck client "default-mouse" raw output enabled SGR mouse reporting
    And deck client "default-mouse" exits cleanly

  @requirement-3-deck-mouse
  Scenario: DECK_MOUSE=0 disables mouse reporting
    Given the deck config disables mouse reporting
    And deck client "no-mouse" is started
    Then deck client "no-mouse" raw output did not enable SGR mouse reporting
    And deck client "no-mouse" exits cleanly

  @requirement-4-fake-agent-sizes
  Scenario Outline: a fake agent records its initial size and every SIGWINCH-observed size
    Given a fake "<agent>" agent is started recording sizes at 40x12
    Then the fake "<agent>" agent recorded sizes are "40x12"
    When the fake "<agent>" agent terminal is resized to 60x20
    Then the fake "<agent>" agent recorded sizes are "40x12,60x20"
    And the fake "<agent>" agent is stopped

    Examples:
      | agent  |
      | claude |
      | pi     |

  @requirement-5-preview-fixtures
  Scenario Outline: a fake agent renders a preview fixture once and then falls silent
    Given a fake "<agent>" agent renders the "<fixture>" preview fixture and falls silent at 40x12
    Then the fake "<agent>" agent's rendered pane is byte-identical across two consecutive captures
    And the fake "<agent>" agent is stopped

    Examples:
      | agent  | fixture     |
      | claude | fitting.txt |
      | claude | oversized.txt |
      | claude | wide.txt    |
      | pi     | fitting.txt |
      | pi     | oversized.txt |
      | pi     | wide.txt    |

  @requirement-7-sidebar-width
  Scenario: sidebar_width can be set and read back for a scenario
    Given deck client "solo" is started
    Then the scenario's persisted sidebar_width is unset
    When the scenario's sidebar_width is set to 50
    Then the scenario's persisted sidebar_width reads back as 50
    When the scenario's sidebar_width is set to 24
    Then the scenario's persisted sidebar_width reads back as 24
    And deck client "solo" exits cleanly
