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
