Feature: A host terminal resize re-fits the preview and the attached pane with no further input (R239, #75)
  SPEC §11.9: when the terminal deck runs in changes size, the selected
  session's tmux window is re-fitted to the new preview box at once, and an
  interactive session's window and grid are resized in the same update. Every
  scenario below resizes the REAL pty of a real deck process and then sends no
  key and no mouse event: the only thing that happens after the resize is time.

  @requirement-239-host-resize
  Scenario: growing and shrinking the host terminal re-fits the preview's window with no further input
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "beacon"
    And within 5 seconds the private tmux window for session "beacon" reports geometry "61x27"
    When deck client "solo" terminal is resized to 120x40
    Then within 5 seconds the private tmux window for session "beacon" reports geometry "81x37"
    When deck client "solo" terminal is resized to 100x30
    Then within 5 seconds the private tmux window for session "beacon" reports geometry "61x27"
    And deck client "solo" exits cleanly

  @requirement-239-host-resize
  Scenario: a resize to the size the terminal already has issues no further resize
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "beacon"
    And within 5 seconds the private tmux window for session "beacon" reports geometry "61x27"
    And 200 milliseconds pass
    When deck client "solo" terminal is resized to 100x30
    And 500 milliseconds pass
    Then the private tmux window for session "beacon" reports geometry "61x27"
    And the fake "claude" agent received exactly 1 SIGWINCH signals
    And deck client "solo" exits cleanly

  @requirement-239-host-resize
  Scenario: growing and shrinking the host terminal resizes the interactive session's pane with no further input
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "beacon"
    And deck client "solo" enters interactive mode
    And within 5 seconds the private tmux window for session "beacon" reports geometry "61x27"
    When deck client "solo" terminal is resized to 120x40
    Then within 5 seconds the private tmux window for session "beacon" reports geometry "81x37"
    When deck client "solo" terminal is resized to 100x30
    Then within 5 seconds the private tmux window for session "beacon" reports geometry "61x27"
    And deck client "solo" leaves interactive mode
    And deck client "solo" exits cleanly
