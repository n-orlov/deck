Feature: Private tmux contract
  deck keeps managed sessions on its configured private tmux socket.

  Scenario: private server options, safe naming, and slug collisions
    Given deck client "contract" is started
    When deck client "contract" creates shell session "Dots.And:Colons"
    Then the private tmux session "deck_dots-and-colons" exists
    And the default tmux socket does not have session "deck_dots-and-colons"
    And the private tmux option "exit-empty" is "off"
    And the private tmux option "remain-on-exit" is "failed"
    And the private tmux option "window-size" is "latest"
    And the private tmux option "aggressive-resize" is "on"
    When deck client "contract" attempts shell session "dots and colons"
    Then deck client "contract" screen contains "name collides with existing slug"
    When deck client "contract" closes the create modal
    And deck client "contract" exits cleanly

  Scenario: missing tmux is actionable
    Given deck client "missing" is started without tmux
    Then deck client "missing" screen contains "tmux unavailable"
    And deck client "missing" screen contains "Install tmux 3.2 or newer"
    When deck client "missing" exits cleanly

  Scenario: old tmux is actionable
    Given deck client "old" is started with tmux version "3.1c"
    Then deck client "old" screen contains "tmux unavailable"
    And deck client "old" screen contains "tmux 3.1c is too old"
    And deck client "old" screen contains "Install tmux 3.2 or newer"
    When deck client "old" exits cleanly
